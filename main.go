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

var (
	chatMessages = make([]string, 0)
	chatMutex    = &sync.RWMutex{}

	userNameStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	timestampStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	pubKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("166"))
	messageStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	messageBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Margin(0, 0, 1, 0)
)

func addMessage(session ssh.Session, message string) {
	chatMutex.Lock()
	defer chatMutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	pubKey := "N/A"
	username := "Anonymous"

	if session != nil {
		username = session.User()
		fmt.Println(session.PublicKey())
		if key := session.PublicKey(); key != nil {
			pubKey = fmt.Sprintf("%x", sha256.Sum256(key.Marshal()))
		}
	}

	// Styled message formatting
	formattedMessage := messageBoxStyle.Render(
		fmt.Sprintf("%s %s %s\n%s",
			userNameStyle.Render(username),
			timestampStyle.Render(timestamp),
			pubKeyStyle.Render("("+pubKey+")"),
			messageStyle.Render(message),
		),
	)

	chatMessages = append(chatMessages, formattedMessage)
}

func getMessages() []string {
	chatMutex.RLock()
	defer chatMutex.RUnlock()
	return append([]string(nil), chatMessages...)
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

	addMessage(nil, "Welcome to the sigmaest Chat Room!")

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
		log.Error("Could not stop server", "error", err)
	}
}

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	pty, _, _ := s.Pty()

	renderer := bubbletea.MakeRenderer(s)

	senderStyle := renderer.NewStyle().Foreground(lipgloss.Color("5"))
	textStyle := renderer.NewStyle().Foreground(lipgloss.Color("10"))
	quitStyle := renderer.NewStyle().Foreground(lipgloss.Color("8"))

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
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.checkNewMessages,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
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
			return m, tea.Quit

		case tea.KeyEnter:
			if m.textarea.Value() != "" {
				addMessage(m.session, m.textarea.Value())

				updatedMessages := getMessages()
				m.viewport.SetContent(m.formatMessages(updatedMessages))
				m.textarea.Reset()
				m.viewport.GotoBottom()
			}
		}

	case messagesUpdatedMsg:
		updatedMessages := getMessages()
		if len(updatedMessages) != m.lastMessageCount {
			m.viewport.SetContent(m.formatMessages(updatedMessages))
			m.viewport.GotoBottom()
			m.lastMessageCount = len(updatedMessages)
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

type messagesUpdatedMsg struct{}

func (m model) checkNewMessages() tea.Msg {
	time.Sleep(refreshInterval)
	return messagesUpdatedMsg{}
}

func (m model) formatMessages(messages []string) string {
	return strings.Join(messages, "\n")
}

func (m model) View() string {
	chatView := fmt.Sprintf(
		"%s\n%s",
		m.viewport.View(),
		m.textarea.View(),
	)

	termInfo := m.textStyle.Render(fmt.Sprintf(
		"Connected as: %s | Term: %s | Window: %dx%d",
		m.session.User(), m.term, m.width, m.height,
	))

	quitInfo := m.quitStyle.Render("Press 'Esc' or 'Ctrl+C' to quit")

	return chatView + "\n" + termInfo + "\n" + quitInfo
}
