package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
)

func localSMTP(t *testing.T) (SMTPOptions, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	messages := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := textproto.NewReader(bufio.NewReader(conn))
		io.WriteString(conn, "220 synthetic mail server\r\n")
		for {
			line, err := reader.ReadLine()
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				io.WriteString(conn, "250-localhost\r\n250 8BITMIME\r\n")
			case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
				io.WriteString(conn, "250 ok\r\n")
			case line == "DATA":
				io.WriteString(conn, "354 send data\r\n")
				data, err := reader.ReadDotBytes()
				if err != nil {
					return
				}
				messages <- string(data)
				io.WriteString(conn, "250 accepted\r\n")
			case line == "QUIT":
				io.WriteString(conn, "221 bye\r\n")
				return
			default:
				io.WriteString(conn, "500 unsupported\r\n")
			}
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return SMTPOptions{Host: host, Port: n, From: "sender@example.test", Security: "plain"}, messages
}

func TestSMTPDeliversMIMEAndDoesNotDowngradeTLS(t *testing.T) {
	options, messages := localSMTP(t)
	sender, err := NewSMTP(options)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(context.Background(), Message{To: "recipient@example.test", Brand: "品牌 A", Subject: "验证邮箱", Text: "仅用于测试的验证邮件"}); err != nil {
		t.Fatal(err)
	}
	message := <-messages
	parts := strings.SplitN(message, "\n\n", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid MIME %q", message)
	}
	body, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(parts[1]), "\n", ""))
	if err != nil || string(body) != "仅用于测试的验证邮件" {
		t.Fatalf("invalid body %q %v", body, err)
	}
	options, _ = localSMTP(t)
	options.Security = "starttls"
	sender, _ = NewSMTP(options)
	if err := sender.Send(context.Background(), Message{To: "recipient@example.test", Subject: "test"}); err == nil {
		t.Fatal("silently downgraded a required TLS connection")
	}
	options.Production = true
	options.Security = "plain"
	if _, err := NewSMTP(options); err == nil {
		t.Fatal("production permitted plaintext SMTP")
	}
	if _, err := NewSMTP(SMTPOptions{Host: "localhost", From: "sender@example.test", Security: "plain", Username: "user", Password: "secret"}); err == nil {
		t.Fatal("credentials permitted on plaintext transport")
	}
}
