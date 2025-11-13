package main

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SMS struct {
	ID        int
	Number    string
	Text      string
	Timestamp time.Time
	State     string
}

type Modem struct {
	id           int
	apn          string
	messages     []SMS
	msgMutex     sync.RWMutex
	subscribers  []chan SMS
	subMutex     sync.RWMutex
	stopMonitor  chan bool
}

func NewModem(id int, apn string) (*Modem, error) {
	// Check if modem exists
	cmd := exec.Command("mmcli", "-m", strconv.Itoa(id))
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("modem %d not found: %w", id, err)
	}

	return &Modem{
		id:          id,
		apn:         apn,
		messages:    make([]SMS, 0),
		subscribers: make([]chan SMS, 0),
		stopMonitor: make(chan bool),
	}, nil
}

func (m *Modem) Connect() error {
	// Enable modem
	cmd := exec.Command("mmcli", "-m", strconv.Itoa(m.id), "-e")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable modem: %w", err)
	}

	// Connect with APN
	connectStr := fmt.Sprintf("apn=%s,ip-type=ipv4", m.apn)
	cmd = exec.Command("mmcli", "-m", strconv.Itoa(m.id), "--simple-connect", connectStr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	return nil
}

func (m *Modem) Disconnect() error {
	close(m.stopMonitor)
	
	cmd := exec.Command("mmcli", "-m", strconv.Itoa(m.id), "--simple-disconnect")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	return nil
}

func (m *Modem) StartSMSMonitoring() {
	// Load existing messages
	m.loadMessages()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopMonitor:
			return
		case <-ticker.C:
			m.checkNewMessages()
		}
	}
}

func (m *Modem) loadMessages() {
	cmd := exec.Command("mmcli", "-m", strconv.Itoa(m.id), "--messaging-list-sms")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to list SMS: %v", err)
		return
	}

	smsIDs := m.parseSMSList(string(output))
	
	m.msgMutex.Lock()
	defer m.msgMutex.Unlock()

	for _, id := range smsIDs {
		if !m.hasMessageID(id) {
			if sms, err := m.getSMS(id); err == nil {
				m.messages = append(m.messages, sms)
			}
		}
	}
}

func (m *Modem) checkNewMessages() {
	cmd := exec.Command("mmcli", "-m", strconv.Itoa(m.id), "--messaging-list-sms")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	smsIDs := m.parseSMSList(string(output))
	
	for _, id := range smsIDs {
		m.msgMutex.RLock()
		hasMsg := m.hasMessageID(id)
		m.msgMutex.RUnlock()

		if !hasMsg {
			if sms, err := m.getSMS(id); err == nil {
				m.msgMutex.Lock()
				m.messages = append(m.messages, sms)
				m.msgMutex.Unlock()

				// Notify subscribers
				m.notifySubscribers(sms)
			}
		}
	}
}

func (m *Modem) parseSMSList(output string) []int {
	re := regexp.MustCompile(`/SMS/(\d+)`)
	matches := re.FindAllStringSubmatch(output, -1)
	
	ids := make([]int, 0)
	for _, match := range matches {
		if len(match) > 1 {
			if id, err := strconv.Atoi(match[1]); err == nil {
				ids = append(ids, id)
			}
		}
	}
	
	return ids
}

func (m *Modem) getSMS(id int) (SMS, error) {
	cmd := exec.Command("mmcli", "-s", strconv.Itoa(id))
	output, err := cmd.Output()
	if err != nil {
		return SMS{}, err
	}

	sms := SMS{
		ID:        id,
		Timestamp: time.Now(),
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		if strings.HasPrefix(line, "number:") {
			sms.Number = strings.TrimSpace(strings.TrimPrefix(line, "number:"))
		} else if strings.HasPrefix(line, "text:") {
			sms.Text = strings.TrimSpace(strings.TrimPrefix(line, "text:"))
		} else if strings.HasPrefix(line, "state:") {
			sms.State = strings.TrimSpace(strings.TrimPrefix(line, "state:"))
		}
	}

	return sms, nil
}

func (m *Modem) hasMessageID(id int) bool {
	for _, msg := range m.messages {
		if msg.ID == id {
			return true
		}
	}
	return false
}

func (m *Modem) GetMessages() []SMS {
	m.msgMutex.RLock()
	defer m.msgMutex.RUnlock()
	
	// Return a copy
	msgs := make([]SMS, len(m.messages))
	copy(msgs, m.messages)
	return msgs
}

func (m *Modem) SendSMS(number, text string) error {
	createCmd := fmt.Sprintf("text='%s',number='%s'", 
		strings.ReplaceAll(text, "'", "\\'"),
		number)
	
	cmd := exec.Command("mmcli", "-m", strconv.Itoa(m.id), 
		"--messaging-create-sms", createCmd)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create SMS: %w", err)
	}

	// Parse SMS ID from output
	re := regexp.MustCompile(`/SMS/(\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return fmt.Errorf("failed to parse SMS ID from output")
	}

	smsID := matches[1]

	// Send the SMS
	cmd = exec.Command("mmcli", "-s", smsID, "--send")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send SMS: %w", err)
	}

	return nil
}

func (m *Modem) Subscribe() chan SMS {
	m.subMutex.Lock()
	defer m.subMutex.Unlock()
	
	ch := make(chan SMS, 10)
	m.subscribers = append(m.subscribers, ch)
	return ch
}

func (m *Modem) Unsubscribe(ch chan SMS) {
	m.subMutex.Lock()
	defer m.subMutex.Unlock()
	
	for i, sub := range m.subscribers {
		if sub == ch {
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
			close(ch)
			break
		}
	}
}

func (m *Modem) notifySubscribers(sms SMS) {
	m.subMutex.RLock()
	defer m.subMutex.RUnlock()
	
	for _, ch := range m.subscribers {
		select {
		case ch <- sms:
		default:
			// Channel full, skip
		}
	}
}

func (m *Modem) GetSignalQuality() (int, error) {
	cmd := exec.Command("mmcli", "-m", strconv.Itoa(m.id))
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	re := regexp.MustCompile(`signal quality:\s*(\d+)%`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return 0, fmt.Errorf("signal quality not found")
	}

	return strconv.Atoi(matches[1])
}
