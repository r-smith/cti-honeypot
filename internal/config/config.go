package config

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// This block of constants defines the application default settings.
const (
	DefaultEnableHTTP           = true
	DefaultEnableHTTPS          = true
	DefaultEnableSSH            = true
	DefaultEnableThreatFeed     = true
	DefaultPortHTTP             = "8080"
	DefaultPortHTTPS            = "8443"
	DefaultPortSSH              = "2022"
	DefaultPortThreatFeed       = "8081"
	DefaultThreatExpiryHours    = 168
	DefaultThreatDatabasePath   = "cti-honeypot-feed.json"
	DefaultThreatIncludePrivate = false
	DefaultLogPath              = "cti-honeypot-log.txt"
	DefaultHtmlPath             = ""
	DefaultCertPathHTTPS        = "cti-honeypot-https.crt"
	DefaultKeyPathHTTPS         = "cti-honeypot-https.key"
	DefaultKeyPathSSH           = "cti-honeypot-ssh.key"
	DefaultBannerSSH            = "SSH-2.0-OpenSSH_9.3 FreeBSD-20230316" // SSH banner for FreeBSD 13.2
)

// ServerType represents the different types of honeypot servers that can be
// deployed. Each type has its own specific handlers and behavior.
type ServerType int

const (
	HTTP ServerType = iota
	HTTPS
	SSH
	TCP
	UDP
)

// String returns a string represenation of ServerType.
func (t ServerType) String() string {
	return [...]string{"http", "https", "ssh", "tcp", "udp"}[t]
}

// UnmarshalXMLAttr unmarshals the XML 'type' attribute from 'server' elements
// into a ServerType.
//
// Example XML snippet:
// <server type="http"><enabled>true</enabled></server>
func (t *ServerType) UnmarshalXMLAttr(attr xml.Attr) error {
	switch attr.Value {
	case "http":
		*t = HTTP
	case "https":
		*t = HTTPS
	case "ssh":
		*t = SSH
	case "tcp":
		*t = TCP
	case "udp":
		*t = UDP
	default:
		return fmt.Errorf("invalid server type: %s", attr.Value)
	}
	return nil
}

// Config holds the configuration settings for the application. It contains the
// logger, settings for managing a threat feed, and the collection of honeypot
// servers that are configured to run.
type Config struct {
	LogPath    string     `xml:"defaultLogPath"`
	Servers    []Server   `xml:"honeypotServers>server"`
	ThreatFeed ThreatFeed `xml:"threatFeed"`
}

// Server represents a honeypot server with its relevant settings.
type Server struct {
	Type       ServerType `xml:"type,attr"`
	Enabled    bool       `xml:"enabled"`
	Port       string     `xml:"port"`
	CertPath   string     `xml:"certPath"`
	KeyPath    string     `xml:"keyPath"`
	HtmlPath   string     `xml:"htmlPath"`
	Banner     string     `xml:"banner"`
	Prompts    []Prompt   `xml:"prompt"`
	LogPath    string     `xml:"logPath"`
	LogEnabled bool       `xml:"logEnabled"`
	LogFile    *os.File
	Logger     *slog.Logger
}

// Prompt represents a text prompt that can be displayed to connecting clients
// when using the TCP-type honeypot server. Each prompt waits for input and
// logs the response. A Server can include multiple prompts which are displayed
// one at a time. The optional Log field gives a description when logging the
// response.
type Prompt struct {
	Text string `xml:",chardata"`
	Log  string `xml:"log,attr"`
}

// ThreatFeed represents an optional HTTP server that serves a list of IP
// addresses observed interacting with your honeypot servers. This server
// outputs data in a format compatible with most enterprise firewalls, which
// can be configured to automatically block communication with IP addresses
// appearing in the threat feed.
type ThreatFeed struct {
	Enabled           bool   `xml:"enabled"`
	Port              string `xml:"port"`
	DatabasePath      string `xml:"databasePath"`
	ExpiryHours       uint   `xml:"threatExpiryHours"`
	IsPrivateIncluded bool   `xml:"isPrivateIncluded"`
	CustomThreatsPath string `xml:"customThreatsPath"`
	ExcludeListPath   string `xml:"excludeListPath"`
}

// Load reads an optional XML configuration file and unmarshals its contents
// into a Config struct. Any errors encountered opening or decoding the file
// are returned. When decoding is successful, the populated Config struct is
// returned.
func Load(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	xmlBytes, _ := io.ReadAll(file)
	err = xml.Unmarshal(xmlBytes, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to decode XML file: %w", err)
	}

	return &config, nil
}

// InitializeLoggers creates structured loggers for each server. It opens log
// files using the server's specified log path, defaulting to the global log
// path if none is provided.
func (c *Config) InitializeLoggers() error {
	for i, srv := range c.Servers {
		if !c.Servers[i].Enabled {
			continue
		}

		// Use the global log path if the server log path is not specified.
		var logPath string
		if len(c.Servers[i].LogPath) > 0 {
			logPath = c.Servers[i].LogPath
		} else {
			logPath = c.LogPath
		}

		// If no log path is given or if logging is disabled, set up a dummy
		// logger to discard output.
		if len(logPath) == 0 || !srv.LogEnabled {
			c.Servers[i].Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			continue
		}

		// Open the specified log file and create a new logger.
		logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0664)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		c.Servers[i].LogFile = logFile
		c.Servers[i].Logger = slog.New(slog.NewJSONHandler(c.Servers[i].LogFile, &slog.HandlerOptions{
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				// Remove 'message' and 'log level' fields from output.
				if a.Key == slog.MessageKey || a.Key == slog.LevelKey {
					return slog.Attr{}
				}
				return a
			},
		}))
	}

	return nil
}

// CloseLogFiles closes all open log file handles for the servers. This
// function should be called when the application is shutting down.
func (c *Config) CloseLogFiles() {
	for i := range c.Servers {
		if c.Servers[i].LogFile != nil {
			c.Servers[i].LogFile.Close()
		}
	}
}
