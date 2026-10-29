// Package mail sends transactional email. It never logs message bodies or credentials.
package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Message struct {
	BrandID string `json:"brand_id,omitempty"`
	ID      string `json:"id,omitempty"`
	To      string `json:"to"`
	Brand   string `json:"brand"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

type Sender interface {
	Send(context.Context, Message) error
}

type SMTPOptions struct {
	Host, Username, Password, From, FromName, Security string
	Port                                               int
	Production                                         bool
}

type SMTP struct{ options SMTPOptions }

func NewSMTP(options SMTPOptions) (*SMTP, error) {
	if options.Security == "" {
		options.Security = "starttls"
	}
	if options.Port == 0 {
		options.Port = 587
	}
	if options.Host == "" || options.Port < 1 || options.Port > 65535 || !ValidAddress(options.From) || strings.ContainsAny(options.FromName, "\r\n") || (options.Username == "") != (options.Password == "") {
		return nil, errors.New("invalid SMTP configuration")
	}
	if options.Security != "tls" && options.Security != "starttls" && (options.Security != "plain" || options.Production || options.Username != "") {
		return nil, errors.New("SMTP requires verified TLS; plain delivery is only for unauthenticated development servers")
	}
	return &SMTP{options: options}, nil
}

func ValidAddress(value string) bool {
	address, err := stdmail.ParseAddress(value)
	return err == nil && address.Address == value && !strings.ContainsAny(value, "\r\n") && len(value) <= 254
}

func (s *SMTP) Send(ctx context.Context, message Message) error {
	if !ValidAddress(message.To) || strings.ContainsAny(message.Brand+message.Subject, "\r\n") || strings.ContainsAny(message.ID, "\r\n<>@ ") {
		return errors.New("invalid mail envelope")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	options := s.options
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(options.Host, strconv.Itoa(options.Port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	if options.Security == "plain" {
		remote, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok || (!remote.IP.IsLoopback() && !remote.IP.IsPrivate()) {
			return errors.New("plaintext SMTP is restricted to local development networks")
		}
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	tlsConfig := &tls.Config{ServerName: options.Host, MinVersion: tls.VersionTLS12}
	if options.Security == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, options.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if options.Security == "starttls" {
		if supported, _ := client.Extension("STARTTLS"); !supported {
			return errors.New("SMTP server does not support required STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if options.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", options.Username, options.Password, options.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(options.From); err != nil {
		return err
	}
	if err := client.Rcpt(message.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	name := options.FromName
	if name == "" {
		name = message.Brand
	}
	from := (&stdmail.Address{Name: name, Address: options.From}).String()
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", from, message.To, mime.QEncoding.Encode("UTF-8", message.Subject), time.Now().UTC().Format(time.RFC1123Z))
	if message.ID != "" {
		_, domain, _ := strings.Cut(options.From, "@")
		body = "Message-ID: <" + message.ID + "@" + domain + ">\r\n" + body
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(message.Text))
	for len(encoded) > 76 {
		body += encoded[:76] + "\r\n"
		encoded = encoded[76:]
	}
	body += encoded + "\r\n"
	if _, err := io.WriteString(writer, body); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	// DATA's final success is the server's acceptance. A subsequent QUIT error
	// must not cause a second copy of an already accepted message.
	_ = client.Quit()
	return nil
}
