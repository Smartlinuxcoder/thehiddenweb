package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
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
	host            = "localhost"
	port            = "23234"
	refreshInterval = 1 * time.Second
)

var (
	chatMessages = make([]string, 0)
	chatMutex    = &sync.RWMutex{}
)

func addMessage(message string) {
	chatMutex.Lock()
	defer chatMutex.Unlock()
	chatMessages = append(chatMessages, message)
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

	addMessage("Welcome to the SSH Chat Room!")

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
				message := m.senderStyle.Render(m.session.User()+": ") + m.textarea.Value()
				addMessage(message)

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
	return lipgloss.JoinVertical(lipgloss.Left, messages...)
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
