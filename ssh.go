package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	gossh "golang.org/x/crypto/ssh"
)

var (
	// Catppuccin Mocha colors
	rosewater = lipgloss.Color("#f5e0dc")
	flamingo  = lipgloss.Color("#f2cdcd")
	pink      = lipgloss.Color("#f5c2e7")
	mauve     = lipgloss.Color("#cba6f7")
	red       = lipgloss.Color("#f38ba8")
	maroon    = lipgloss.Color("#eba0ac")
	peach     = lipgloss.Color("#fab387")
	yellow    = lipgloss.Color("#f9e2af")
	green     = lipgloss.Color("#a6e3a1")
	teal      = lipgloss.Color("#94e2d5")
	sky       = lipgloss.Color("#89dceb")
	sapphire  = lipgloss.Color("#74c7ec")
	blue      = lipgloss.Color("#89b4fa")
	lavender  = lipgloss.Color("#b4befe")
	text      = lipgloss.Color("#cdd6f4")
	subtext1  = lipgloss.Color("#bac2de")
	subtext0  = lipgloss.Color("#a6adc8")
	overlay2  = lipgloss.Color("#9399b2")
	overlay1  = lipgloss.Color("#7f849c")
	overlay0  = lipgloss.Color("#6c7086")
	surface2  = lipgloss.Color("#585b70")
	surface1  = lipgloss.Color("#45475a")
	surface0  = lipgloss.Color("#313244")
	base      = lipgloss.Color("#1e1e2e")
	mantle    = lipgloss.Color("#181825")
	crust     = lipgloss.Color("#11111b")
)

type SSHServer struct {
	server *ssh.Server
	modem  *Modem
}

func NewSSHServer(port int, hostKeyPath string, modem *Modem) (*SSHServer, error) {
	// Expand home directory
	if strings.HasPrefix(hostKeyPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		hostKeyPath = filepath.Join(home, hostKeyPath[2:])
	}

	s, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf(":%d", port)),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithPublicKeyAuth(publicKeyHandler),
		wish.WithMiddleware(
			bubbletea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				pty, _, active := s.Pty()
				if !active {
					return nil, nil
				}

				m := initialModel(modem, pty.Window.Width, pty.Window.Height)
				return m, []tea.ProgramOption{tea.WithAltScreen()}
			}),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		return nil, err
	}

	return &SSHServer{
		server: s,
		modem:  modem,
	}, nil
}

func publicKeyHandler(ctx ssh.Context, key ssh.PublicKey) bool {
	// Get the actual user's home, not root's
	user := ctx.User()

	// Try to load from the connecting user's home
	authKeysPath := filepath.Join("/home", user, ".ssh", "authorized_keys")
	data, err := os.ReadFile(authKeysPath)
	if err != nil {
		// Fallback to root's authorized_keys if running as root
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		authKeysPath = filepath.Join(home, ".ssh", "authorized_keys")
		data, err = os.ReadFile(authKeysPath)
		if err != nil {
			return false
		}
	}

	// Parse authorized keys
	for len(data) > 0 {
		pubKey, _, _, rest, err := gossh.ParseAuthorizedKey(data)
		if err != nil {
			data = rest
			continue
		}

		// Compare key fingerprints
		if string(key.Marshal()) == string(pubKey.Marshal()) {
			return true
		}

		data = rest
	}

	return false
}

func (s *SSHServer) Start() error {
	return s.server.ListenAndServe()
}

func (s *SSHServer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.server.Shutdown(ctx)
}

type viewMode int

const (
	viewList viewMode = iota
	viewMessage
	viewCompose
)

type smsItem struct {
	sms SMS
}

func (i smsItem) Title() string {
	return i.sms.Number
}

func (i smsItem) Description() string {
	preview := i.sms.Text
	if len(preview) > 50 {
		preview = preview[:47] + "..."
	}
	// Ensure it's not empty string
	if preview == "" {
		preview = "(empty message)"
	}
	return preview
}

func (i smsItem) FilterValue() string {
	return i.sms.Number + " " + i.sms.Text
}

type model struct {
	modem       *Modem
	mode        viewMode
	list        list.Model
	msgView     string
	numberInput textinput.Model
	textInput   textarea.Model
	width       int
	height      int
	smsChan     chan SMS
	err         error
}

type newSMSMsg SMS

func waitForSMS(sub chan SMS) tea.Cmd {
	return func() tea.Msg {
		sms := <-sub
		return newSMSMsg(sms)
	}
}

func initialModel(modem *Modem, width, height int) model {
	// Create list
	messages := modem.GetMessages()
	items := make([]list.Item, len(messages))
	for i, msg := range messages {
		items[i] = smsItem{sms: msg}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(mauve).
		BorderForeground(mauve)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(subtext1).
		BorderForeground(mauve)

	l := list.New(items, delegate, width, height-4)
	l.Title = "☎ landline sms"
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(pink).
		Bold(true).
		Padding(0, 1)
	l.Styles.TitleBar = lipgloss.NewStyle().
		Background(surface0).
		Foreground(text)

	// Create number input
	ni := textinput.New()
	ni.Placeholder = "+1234567890"
	ni.Focus()
	ni.CharLimit = 20
	ni.Width = 30
	ni.PromptStyle = lipgloss.NewStyle().Foreground(blue)
	ni.TextStyle = lipgloss.NewStyle().Foreground(text)

	// Create text area
	ti := textarea.New()
	ti.Placeholder = "Type your message..."
	ti.CharLimit = 160
	ti.SetWidth(width - 4)
	ti.SetHeight(5)
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ti.ShowLineNumbers = false

	// Subscribe to new messages
	smsChan := modem.Subscribe()

	return model{
		modem:       modem,
		mode:        viewList,
		list:        l,
		numberInput: ni,
		textInput:   ti,
		width:       width,
		height:      height,
		smsChan:     smsChan,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		waitForSMS(m.smsChan),
		tea.EnterAltScreen,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		m.textInput.SetWidth(msg.Width - 4)
		return m, nil

	case newSMSMsg:
		// Add new message to list
		sms := SMS(msg)
		m.list.InsertItem(0, smsItem{sms: sms})
		return m, waitForSMS(m.smsChan)

	case tea.KeyMsg:
		switch m.mode {
		case viewList:
			switch msg.String() {
			case "q", "ctrl+c":
				m.modem.Unsubscribe(m.smsChan)
				return m, tea.Quit
			case "n":
				m.mode = viewCompose
				m.numberInput.Focus()
				return m, textinput.Blink
			case "enter":
				if item, ok := m.list.SelectedItem().(smsItem); ok {
					m.mode = viewMessage
					m.msgView = m.formatMessage(item.sms)
				}
				return m, nil
			}

		case viewMessage:
			if msg.String() == "esc" || msg.String() == "q" {
				m.mode = viewList
				return m, nil
			}

		case viewCompose:
			switch msg.String() {
			case "esc":
				m.mode = viewList
				m.numberInput.SetValue("")
				m.textInput.SetValue("")
				m.numberInput.Focus()
				return m, nil
			case "ctrl+s":
				// Send SMS
				number := m.numberInput.Value()
				text := m.textInput.Value()
				if number != "" && text != "" {
					if err := m.modem.SendSMS(number, text); err != nil {
						m.err = err
					} else {
						m.mode = viewList
						m.numberInput.SetValue("")
						m.textInput.SetValue("")
						m.err = nil
					}
				}
				return m, nil
			case "tab", "shift+tab":
				if m.numberInput.Focused() {
					m.numberInput.Blur()
					m.textInput.Focus()
				} else {
					m.textInput.Blur()
					m.numberInput.Focus()
				}
				return m, nil
			}
		}
	}

	// Handle updates for active components
	var cmd tea.Cmd
	switch m.mode {
	case viewList:
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	case viewCompose:
		if m.numberInput.Focused() {
			m.numberInput, cmd = m.numberInput.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.textInput.Focused() {
			m.textInput, cmd = m.textInput.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	switch m.mode {
	case viewList:
		return m.viewList()
	case viewMessage:
		return m.viewMessage()
	case viewCompose:
		return m.viewCompose()
	}
	return ""
}

func (m model) viewList() string {
	statusStyle := lipgloss.NewStyle().
		Foreground(subtext0).
		Background(surface0).
		Padding(0, 1)

	signal, _ := m.modem.GetSignalQuality()
	status := fmt.Sprintf(" 📶 %d%% | n: new | enter: view | q: quit ", signal)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.list.View(),
		statusStyle.Render(status),
	)
}

func (m model) viewMessage() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(pink).
		Bold(true).
		Padding(1, 2).
		Background(surface0)

	contentStyle := lipgloss.NewStyle().
		Foreground(text).
		Padding(1, 2)

	helpStyle := lipgloss.NewStyle().
		Foreground(subtext0).
		Background(surface0).
		Padding(0, 1)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("☎ message"),
		contentStyle.Render(m.msgView),
		helpStyle.Render(" esc: back "),
	)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Left,
		lipgloss.Top,
		content,
	)
}

func (m model) viewCompose() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Padding(1, 2).
		Background(surface0)

	labelStyle := lipgloss.NewStyle().
		Foreground(blue).
		Bold(true)

	helpStyle := lipgloss.NewStyle().
		Foreground(subtext0).
		Background(surface0).
		Padding(0, 1)

	errStyle := lipgloss.NewStyle().
		Foreground(red).
		Padding(0, 2)

	var errMsg string
	if m.err != nil {
		errMsg = errStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("✉️  new message"),
		"",
		labelStyle.Render("  to:"),
		"  "+m.numberInput.View(),
		"",
		labelStyle.Render("  message:"),
		"  "+m.textInput.View(),
		"",
		errMsg,
		helpStyle.Render(" tab: switch field | ctrl+s: send | esc: cancel "),
	)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Left,
		lipgloss.Top,
		content,
	)
}

func (m model) formatMessage(sms SMS) string {
	fromStyle := lipgloss.NewStyle().
		Foreground(lavender).
		Bold(true)

	numberStyle := lipgloss.NewStyle().
		Foreground(blue).
		Bold(true)

	timeStyle := lipgloss.NewStyle().
		Foreground(overlay2)

	textStyle := lipgloss.NewStyle().
		Foreground(text).
		Bold(false).
		Padding(1, 0)

	// Debug: ensure text isn't empty
	messageText := sms.Text
	if messageText == "" {
		messageText = "(no message text)"
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		fromStyle.Render("from: ")+numberStyle.Render(sms.Number),
		timeStyle.Render(sms.Timestamp.Format("Jan 02, 2006 15:04")),
		"",
		textStyle.Render(messageText),
	)
}
