# 🕾 landline

A secure SSH-accessible SMS gateway for receiving 2FA codes without needing your phone.

## Features

- SSH access with Charm Wish
- Send and receive SMS messages
- Beautiful TUI with Catppuccin Mocha theme
- Live updates when new messages arrive
- Signal strength monitoring
- ModemManager integration

## Requirements

- Go 1.21 or later
- ModemManager
- A compatible cellular modem (tested with SIM7600G-H)
- Linux system with systemd (Raspberry Pi, etc.)

## Installation

```bash
# Clone the repository
git clone https://github.com/operatorfoundation/landline.git
cd landline

# Build
go build -o landline

# Install (optional)
sudo cp landline /usr/local/bin/
```

## Usage

```bash
# Run with defaults
sudo ./landline

# Specify custom options
sudo ./landline -port 2222 -modem 0 -apn RESELLER

# Show help
./landline -h
```

### Command-line Options

- `-port` - SSH server port (default: 2222)
- `-hostkey` - Path to SSH host key (default: ~/.ssh/landline_host_key)
- `-modem` - ModemManager modem ID (default: 0)
- `-apn` - APN for cellular connection (default: RESELLER)
- `-debug` - Enable debug logging
- `-version` - Show version

## Connecting

```bash
# Connect via SSH
ssh -p 2222 localhost

# Or from another machine
ssh -p 2222 user@your-pi-ip
```

## TUI Controls

### Message List View
- `↑/↓` or `j/k` - Navigate messages
- `Enter` - View selected message
- `n` - Compose new message
- `q` or `Ctrl+C` - Quit

### Message View
- `Esc` or `q` - Back to list

### Compose View
- `Tab` - Switch between number and message fields
- `Ctrl+S` - Send message
- `Esc` - Cancel

## Running as a Service

Create `/etc/systemd/system/landline.service`:

```ini
[Unit]
Description=Landline SMS Gateway
After=network.target ModemManager.service

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/landline -port 2222 -apn RESELLER
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable landline
sudo systemctl start landline
sudo systemctl status landline
```

## Security Notes

- By default, Landline accepts any SSH key. Consider adding authentication middleware for production use
- The host key is automatically generated on first run
- Run behind Tailscale or a VPN for secure remote access
- Consider using SSH key-based authentication only

## Troubleshooting

### Modem not found
```bash
# List available modems
mmcli -L

# Check modem status
mmcli -m 0
```

### SMS not receiving
```bash
# Check signal quality
mmcli -m 0 | grep signal

# List SMS manually
mmcli -m 0 --messaging-list-sms
```

### Connection issues
```bash
# Check if modem is connected
mmcli -m 0 | grep state

# Reconnect manually
mmcli -m 0 --simple-disconnect
mmcli -m 0 --simple-connect="apn=RESELLER"
```

## License

MIT

## Author

Built for the Operator Foundation
