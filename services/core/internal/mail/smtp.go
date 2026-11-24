package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// SMTPConfig is how to reach a provider's SMTP relay (Brevo, Mailgun, Resend and others all offer one).
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string // never logged
	From     netmail.Address
}

// SMTPMailer sends through an SMTP relay. It always encrypts before it sends the password:
// port 465 starts encrypted, any other port must offer STARTTLS or the send is refused.
type SMTPMailer struct {
	cfg   SMTPConfig
	roots *x509.CertPool // nil means the system roots; a test points it at its own certificate
}

// NewSMTP checks the settings and returns a mailer. It does not connect.
func NewSMTP(cfg SMTPConfig) (*SMTPMailer, error) {
	switch {
	case cfg.Host == "" || strings.ContainsAny(cfg.Host, " /\\@:\r\n"):
		return nil, errors.New("the smtp host must be a bare host name")
	case cfg.Port < 1 || cfg.Port > 65535:
		return nil, errors.New("the smtp port must be between 1 and 65535")
	case cfg.Username == "" || cfg.Password == "":
		return nil, errors.New("the smtp username and password are required")
	case cfg.From.Address == "" || hasLineBreak(cfg.From.Address) || hasLineBreak(cfg.From.Name):
		return nil, errors.New("the from address is required")
	}
	return &SMTPMailer{cfg: cfg}, nil
}

func hasLineBreak(s string) bool { return strings.ContainsAny(s, "\r\n") }

// Send delivers one message. Errors name the step and the server's numeric reply, never the
// server's text, which can repeat the recipient's address, and never a credential.
func (s *SMTPMailer) Send(ctx context.Context, m Message) error {
	to, err := netmail.ParseAddress(m.To)
	if err != nil || to.Name != "" || hasLineBreak(m.To) {
		return errors.New("smtp: the recipient address is not valid")
	}
	if hasLineBreak(m.Subject) {
		return errors.New("smtp: the subject must be one line")
	}
	body, err := s.compose(to.Address, m)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return errors.New("smtp: could not connect")
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() }) // a cancelled send must not hang on the wire
	defer stop()

	tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12, RootCAs: s.roots}
	if s.cfg.Port == 465 {
		conn = tls.Client(conn, tlsCfg)
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return step("greeting", err)
	}
	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("smtp: the server does not offer STARTTLS, so the password was not sent")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return step("starttls", err)
		}
	}
	if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
		return step("login", err)
	}
	if err := c.Mail(s.cfg.From.Address); err != nil {
		return step("sender", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return step("recipient", err)
	}
	w, err := c.Data()
	if err != nil {
		return step("data", err)
	}
	if _, err := w.Write(body); err != nil {
		return step("data", err)
	}
	if err := w.Close(); err != nil {
		return step("data", err)
	}
	_ = c.Quit()
	return nil
}

// step reports where it failed and the server's reply code, nothing it said.
func step(stage string, err error) error {
	var te *textproto.Error
	if errors.As(err, &te) {
		return fmt.Errorf("smtp: %s refused (reply %d)", stage, te.Code)
	}
	return fmt.Errorf("smtp: %s failed", stage)
}

func (s *SMTPMailer) compose(to string, m Message) ([]byte, error) {
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("smtp: %w", err)
	}
	domain := "localhost"
	if at := strings.LastIndex(s.cfg.From.Address, "@"); at >= 0 {
		domain = s.cfg.From.Address[at+1:]
	}
	var b bytes.Buffer
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", s.cfg.From.String())
	h("To", (&netmail.Address{Address: to}).String())
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", time.Now().UTC().Format(time.RFC1123Z))
	h("Message-ID", fmt.Sprintf("<%x@%s>", id, domain))
	h("MIME-Version", "1.0")
	if m.HTML == "" {
		h("Content-Type", `text/plain; charset="utf-8"`)
		h("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		if err := writeQP(&b, m.Text); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}
	// Both versions: clients show the last part they can draw, so plain text goes first.
	boundary := fmt.Sprintf("readmely-%x", id)
	h("Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary))
	b.WriteString("\r\n")
	for _, part := range []struct{ kind, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		b.WriteString("--" + boundary + "\r\n")
		fmt.Fprintf(&b, "Content-Type: %s; charset=\"utf-8\"\r\n", part.kind)
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		if err := writeQP(&b, part.body); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return b.Bytes(), nil
}

func writeQP(b *bytes.Buffer, body string) error {
	qp := quotedprintable.NewWriter(b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(body, "\r\n", "\n"))); err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	if err := qp.Close(); err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	return nil
}
