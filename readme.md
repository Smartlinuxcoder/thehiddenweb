# LetsGoSky.social

### Connection
```bash
ssh letsgosky.social
```

## Overview

LetsGoSky is a terminal-based social platform, no browser needed!.

### Interaction Modes

#### Input Mode (Default)
- Type and send messages
- Press `TAB` to enter selection mode

#### Selection Mode
- Navigate messages with ↑/↓
- Upvote with `u`
- Downvote with `d`
- Press `TAB` to return to input mode

### Quitting
- Press `Esc` or `Ctrl+C`


## Features

### Messaging
- Real-time chat messaging
- 280-character message limit
- Instant message broadcasting to all connected users

### User Interaction
- SSH key-based authentication
- Anonymous username support
- Message voting system (upvotes and downvotes)
- Message selection and interaction mode

### User Experience
- Terminal-based user interface
- Responsive design adapting to terminal window size
- Color-coded messages and UI elements
- System message notifications
- User connection/disconnection alerts
- Online user count display

### Technical Capabilities
- Concurrent user support
- Mutex-protected shared state
- Unique message identification
- Vote tracking per message
- Prevention of duplicate voting

## Technical Details

### Technology Stack
- **Language**: Go (Golang)
- **Libraries**:
  - `charmbracelet/wish` for SSH server
  - `charmbracelet/bubbletea` for Terminal UI
  - `charmbracelet/lipgloss` for styling

### Architecture
- SSH-based server
- Concurrent connection handling
- In-memory message storage
- Thread-safe operation YaY

### Security
- SSH key authentication
- nothing else

## Getting Started

### Prerequisites
- Go 1.16+
- SSH client


## Server Setup

1. Clone the repository
2. Run: `go run main.go`

## Contributing
Contributions are welcome. Please open issues or submit pull requests.

## License
GNU GPL v3