package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

const (
	host            = "0.0.0.0"
	port            = "23234"
	refreshInterval = 1 * time.Second
)

type Message struct {
	Username  string
	Timestamp string
	PubKey    string
	Content   string
	Upvotes   int
	Downvotes int
	UniqueID  string
	System    bool
}

var (
	chatMessages = make([]Message, 0)
	chatMutex    = &sync.RWMutex{}
	usersMutex   = &sync.Mutex{}
	onlineUsers  = 0
	userVotes    = make(map[string]map[string]int)
	voteMutex    = &sync.Mutex{}

	// Catppuccin Mocha color palette
	base     = lipgloss.Color("#1E1E2E")
	text     = lipgloss.Color("#CDD6F4")
	subtext0 = lipgloss.Color("#A6ADC8")
	blue     = lipgloss.Color("#89B4FA")
	green    = lipgloss.Color("#A6E3A1")
	lavender = lipgloss.Color("#B4BEFE")
	peach    = lipgloss.Color("#FAB387")
	red      = lipgloss.Color("#F38BA8")
	selected = lipgloss.Color("#45475A")

	userNameStyle   = lipgloss.NewStyle().Foreground(blue).Bold(true)
	timestampStyle  = lipgloss.NewStyle().Foreground(subtext0).Italic(true)
	pubKeyStyle     = lipgloss.NewStyle().Foreground(peach)
	messageStyle    = lipgloss.NewStyle().Foreground(text)
	upvoteStyle     = lipgloss.NewStyle().Foreground(green)
	downvoteStyle   = lipgloss.NewStyle().Foreground(red)
	messageBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtext0).
			Padding(0, 1).
			Margin(0, 0, 1, 0)
)

func incrementUsers() {
	fmt.Println("incrementing users")
	usersMutex.Lock()
	defer usersMutex.Unlock()
	onlineUsers++
}

func decrementUsers() {
	fmt.Println("decrementing users")
	usersMutex.Lock()
	defer usersMutex.Unlock()
	if onlineUsers > 0 {
		onlineUsers--
	}
}

func generateUniqueMessageID(msg Message) string {
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s%s%s", msg.Username, msg.Timestamp, msg.Content)))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func voteMessage(session ssh.Session, messageID string, voteType int) error {
	voteMutex.Lock()
	defer voteMutex.Unlock()

	username := session.User()
	userVotesForMessage, exists := userVotes[username]
	if !exists {
		userVotes[username] = make(map[string]int)
		userVotesForMessage = userVotes[username]
	}

	// Check if user has already voted this way
	if userVotesForMessage[messageID] == voteType {
		return errors.New("you have already voted this way")
	}

	chatMutex.Lock()
	defer chatMutex.Unlock()

	for i, msg := range chatMessages {
		if msg.UniqueID == messageID {
			// Remove previous vote if exists
			if prevVote, exists := userVotesForMessage[messageID]; exists {
				if prevVote > 0 {
					chatMessages[i].Upvotes--
				} else if prevVote < 0 {
					chatMessages[i].Downvotes--
				}
			}

			// Add new vote
			if voteType > 0 {
				chatMessages[i].Upvotes++
			} else {
				chatMessages[i].Downvotes++
			}

			// Update user's vote
			userVotesForMessage[messageID] = voteType
			return nil
		}
	}

	return errors.New("message not found")
}

func addMessage(session ssh.Session, message string, system bool) {
	chatMutex.Lock()
	defer chatMutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	pubKey := "N/A"
	username := "Anonymous"

	if session != nil {
		username = session.User()
		if key := session.PublicKey(); key != nil {
			pubKey = fmt.Sprintf("%x", sha256.Sum256(key.Marshal()))
		}
	}

	msg := Message{
		Username:  username,
		Timestamp: timestamp,
		PubKey:    pubKey,
		Content:   message,
		Upvotes:   0,
		Downvotes: 0,
		System:    system,
	}

	if !system {
		msg.UniqueID = generateUniqueMessageID(msg)
	}

	chatMessages = append(chatMessages, msg)
}

func getMessages() []Message {
	chatMutex.RLock()
	defer chatMutex.RUnlock()
	return append([]Message(nil), chatMessages...)
}

func main() {
	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Error("Could not start server", "error", err)
		return
	}

	addMessage(nil, "Welcome to letsgosky.social", true)

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("Starting SSH chat server", "host", host, "port", port)

	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Info("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() { cancel() }()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Could not stop server somehow", "error", err)
	}
}

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	addMessage(nil, fmt.Sprintf("Silly goober %s has made the mistake of joining the cult", s.User()), true)

	incrementUsers()

	pty, _, _ := s.Pty()

	renderer := bubbletea.MakeRenderer(s)

	senderStyle := renderer.NewStyle().Foreground(lavender)
	textStyle := renderer.NewStyle().Foreground(green)
	quitStyle := renderer.NewStyle().Foreground(subtext0)

	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()
	ta.Prompt = "┃ "
	ta.CharLimit = 280
	ta.SetWidth(pty.Window.Width - 2)
	ta.SetHeight(3)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false

	vp := viewport.New(pty.Window.Width, pty.Window.Height-5)

	existingMessages := getMessages()

	m := model{
		session:     s,
		viewport:    vp,
		textarea:    ta,
		senderStyle: senderStyle,
		textStyle:   textStyle,
		quitStyle:   quitStyle,
		term:        pty.Term,
		width:       pty.Window.Width,
		height:      pty.Window.Height,
	}

	m.viewport.SetContent(m.formatMessages(existingMessages))
	m.viewport.GotoBottom()

	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

type model struct {
	session          ssh.Session
	viewport         viewport.Model
	textarea         textarea.Model
	senderStyle      lipgloss.Style
	textStyle        lipgloss.Style
	quitStyle        lipgloss.Style
	term             string
	width            int
	height           int
	err              error
	lastMessageCount int
	selectedMessage  int
	isSelectMode     bool
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.checkNewMessages,
	)
}

type messagesUpdatedMsg struct{}

func (m model) checkNewMessages() tea.Msg {
	time.Sleep(refreshInterval)
	return messagesUpdatedMsg{}
}
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	// Handle different input modes
	if !m.isSelectMode {
		m.textarea, tiCmd = m.textarea.Update(msg)
	}
	m.viewport, vpCmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		m.textarea.SetWidth(msg.Width - 2)
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 5

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			decrementUsers()
			return m, tea.Quit

		case tea.KeyTab:
			// Toggle between input and selection modes
			m.isSelectMode = !m.isSelectMode
			if m.isSelectMode {
				m.textarea.Blur()
				// Initialize selected message to the last message
				m.selectedMessage = len(chatMessages) - 1
			} else {
				m.textarea.Focus()
			}

		case tea.KeyEnter:
			if !m.isSelectMode && m.textarea.Value() != "" {
				addMessage(m.session, m.textarea.Value(), false)

				updatedMessages := getMessages()
				m.viewport.SetContent(m.formatMessages(updatedMessages))
				m.textarea.Reset()
				m.viewport.GotoBottom()
			}

		default:
			if m.isSelectMode {
				switch msg.Type {
				case tea.KeyUp:
					for {
						if m.selectedMessage > 0 {
							m.selectedMessage--
						}
						if m.selectedMessage == 0 || !chatMessages[m.selectedMessage].System {
							break
						}
					}
					m.viewport.SetContent(m.formatMessages(getMessages()))

				case tea.KeyDown:
					for {
						if m.selectedMessage < len(chatMessages)-1 {
							m.selectedMessage++
						}
						if m.selectedMessage == len(chatMessages)-1 || !chatMessages[m.selectedMessage].System {
							break
						}
					}
					m.viewport.SetContent(m.formatMessages(getMessages()))

				case tea.KeyRunes:
					switch msg.String() {
					case "u", "U":
						if len(chatMessages) > m.selectedMessage {
							selectedMsg := chatMessages[m.selectedMessage]
							err := voteMessage(m.session, selectedMsg.UniqueID, 1)
							if err != nil {
								m.err = err
							} else {
								updatedMessages := getMessages()
								m.viewport.SetContent(m.formatMessages(updatedMessages))
							}
						}

					case "d", "D":
						if len(chatMessages) > m.selectedMessage {
							selectedMsg := chatMessages[m.selectedMessage]
							err := voteMessage(m.session, selectedMsg.UniqueID, -1)
							if err != nil {
								m.err = err
							} else {
								updatedMessages := getMessages()
								m.viewport.SetContent(m.formatMessages(updatedMessages))
							}
						}
					}
				}
			}
		}

	case messagesUpdatedMsg:
		updatedMessages := getMessages()
		if len(updatedMessages) != m.lastMessageCount {
			m.viewport.SetContent(m.formatMessages(updatedMessages))
			m.viewport.GotoBottom()
			m.lastMessageCount = len(updatedMessages)

			m.selectedMessage = len(updatedMessages) - 1
		}

	case error:
		m.err = msg
		return m, nil
	}

	return m, tea.Batch(
		tiCmd,
		vpCmd,
		m.checkNewMessages,
	)
}
func (m model) formatMessages(messages []Message) string {
	var formattedMessages []string
	for i, msg := range messages {
		if m.isSelectMode && msg.System {
			continue // Skip system messages in selection mode
		}

		var messageStyle lipgloss.Style
		var boxStyle lipgloss.Style

		if msg.System {
			// Style for system messages
			messageStyle = lipgloss.NewStyle().Foreground(subtext0).Italic(true)
			formattedMessages = append(formattedMessages, messageStyle.Render(msg.Content))
			continue
		}

		if m.isSelectMode && i == m.selectedMessage {
			boxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(red).
				Padding(0, 1).
				Margin(0, 0, 1, 0)

			messageStyle = lipgloss.NewStyle().
				Background(selected).
				Foreground(text).
				Padding(0, 1)
		} else {
			boxStyle = messageBoxStyle
			messageStyle = lipgloss.NewStyle().
				Foreground(text)
		}

		header := fmt.Sprintf("%s %s %s | %s %s",
			userNameStyle.Render(msg.Username),
			timestampStyle.Render(msg.Timestamp),
			pubKeyStyle.Render("("+msg.PubKey+")"),
			upvoteStyle.Render(fmt.Sprintf("👍 %d", msg.Upvotes)),
			downvoteStyle.Render(fmt.Sprintf("👎 %d", msg.Downvotes)),
		)

		formattedMessage := boxStyle.Render(
			header + "\n" + messageStyle.Render(msg.Content),
		)
		formattedMessages = append(formattedMessages, formattedMessage)
	}
	return strings.Join(formattedMessages, "\n")
}

func (m model) View() string {
	chatView := fmt.Sprintf(
		"%s\n%s",
		m.viewport.View(),
		m.textarea.View(),
	)

	termInfo := m.textStyle.Render(fmt.Sprintf(
		"Connected as: %s | Term: %s | Window: %dx%d | Online users: %d",
		m.session.User(), m.term, m.width, m.height, onlineUsers,
	))

	var modeInfo string
	if m.isSelectMode {
		modeInfo = m.quitStyle.Render(
			"SELECTION MODE: ↑/↓ to navigate | 'u' to upvote | 'd' to downvote | TAB to exit",
		)
	} else {
		modeInfo = m.quitStyle.Render(
			"INPUT MODE: Type message | TAB to select messages | 'Esc' or 'Ctrl+C' to quit",
		)
	}

	return chatView + "\n" + termInfo + "\n" + modeInfo
}
