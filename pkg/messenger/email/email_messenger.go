package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"strings"

	"github.com/golang/glog"
	"github.com/sbezverk/routercommander/pkg/messenger"
)

var _ messenger.Notifier = &eMessenger{}

type eMessenger struct {
	server string
	user   string
	pass   string
	from   string
	to     []string
}

func (em *eMessenger) Notify(fn string, b []byte) error {
	host, _, _ := net.SplitHostPort(em.server)
	ua := smtp.PlainAuth("routercommander", em.user, em.pass, host)

	// TLS config
	// tlsconfig := &tls.Config{
	// 	InsecureSkipVerify: true,
	// 	ServerName:         host,
	// }

	// conn, err := tls.Dial("tcp", em.server, tlsconfig)
	// if err != nil {
	// 	return fmt.Errorf("tls Dial failed with error: %+v", err)
	// }
	// defer conn.Close()

	// mc, err := smtp.NewClient(conn, host)
	// if err != nil {
	// 	return fmt.Errorf("SMTP New Client failed with error: %+v", err)
	// }

	msg, err := NewMailMessage(em.to, []string{}, []string{}, fmt.Sprintf("routercommander log %s", fn), fmt.Sprintf("see attached log: %s", fn), map[string][]byte{
		fn: b,
	})
	if err != nil {
		return err
	}

	if err := smtp.SendMail(em.server, ua, em.from, em.to, msg.MarshalBytes()); err != nil {
		return fmt.Errorf("SendMail failed with error: %+v", err)
	}
	// // Auth
	// if err = mc.Auth(ua); err != nil {
	// 	return fmt.Errorf("SMTP Client Auth failed with error: %+v", err)
	// }

	// // To && From
	// if err = mc.Mail(em.from); err != nil {
	// 	return fmt.Errorf("SMTP Client Mail call failed with error: %+v", err)
	// }

	// if err = mc.Rcpt(em.to[0]); err != nil {
	// 	return fmt.Errorf("SMTP Client Rcpt call failed with error: %+v", err)
	// }

	// // Data
	// w, err := mc.Data()
	// if err != nil {
	// 	return fmt.Errorf("SMTP Client Data call failed with error: %+v", err)
	// }

	// _, err = w.Write(msg.MarshalBytes())
	// if err != nil {
	// 	return fmt.Errorf("SMTP Client Write call failed with error: %+v", err)
	// }

	// err = w.Close()
	// if err != nil {
	// 	return fmt.Errorf("SMTP Client Close call failed with error: %+v", err)
	// }
	// mc.Quit()

	return nil
}

func NewEmailNotifier(smtp, user, pass, from string, to string) (messenger.Notifier, error) {
	if len(strings.Split(smtp, ":")) < 2 {
		return nil, fmt.Errorf("server address %s must include smtp port", smtp)
	}
	em := &eMessenger{
		server: smtp,
		user:   user,
		pass:   pass,
		from:   from,
		to:     make([]string, 0),
	}
	tos := strings.Split(to, ",")
	if len(tos) < 1 {
		return nil, fmt.Errorf("destination email address(es) list cannot be empty")
	}

	for i := 0; i < len(tos); i++ {
		em.to = append(em.to, tos[i])
	}

	glog.Infof("email notifier has been instantiated successfully.")
	return em, nil
}

type MailMessage struct {
	To          []string
	CC          []string
	BCC         []string
	Subject     string
	Body        string
	Attachments map[string][]byte
}

func (mm *MailMessage) MarshalBytes() []byte {
	buf := bytes.NewBuffer(nil)
	withAttachments := len(mm.Attachments) > 0
	buf.WriteString(fmt.Sprintf("Subject: %s\n", mm.Subject))
	buf.WriteString(fmt.Sprintf("To: %s\n", strings.Join(mm.To, ",")))
	if len(mm.CC) > 0 {
		buf.WriteString(fmt.Sprintf("Cc: %s\n", strings.Join(mm.CC, ",")))
	}

	if len(mm.BCC) > 0 {
		buf.WriteString(fmt.Sprintf("Bcc: %s\n", strings.Join(mm.BCC, ",")))
	}

	buf.WriteString("MIME-Version: 1.0\n")
	writer := multipart.NewWriter(buf)
	boundary := writer.Boundary()
	if withAttachments {
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\n", boundary))
		buf.WriteString(fmt.Sprintf("--%s\n", boundary))
	} else {
		buf.WriteString("Content-Type: text/plain; charset=utf-8\n")
	}

	buf.WriteString("\n" + mm.Body + "\n")
	if withAttachments {
		for k, v := range mm.Attachments {
			buf.WriteString(fmt.Sprintf("\n\n--%s\n", boundary))
			buf.WriteString(fmt.Sprintf("Content-Type: %s\n", http.DetectContentType(v)))
			buf.WriteString("Content-Transfer-Encoding: base64\n")
			buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=%s\n\n", k))
			b := make([]byte, base64.StdEncoding.EncodedLen(len(v)))
			base64.StdEncoding.Encode(b, v)
			buf.Write(b)
			buf.WriteString(fmt.Sprintf("\n--%s", boundary))
		}
		buf.WriteString("--")
	}

	return buf.Bytes()
}

func NewMailMessage(to []string, cc []string, bcc []string, subject string, body string, attachments map[string][]byte) (*MailMessage, error) {
	mm := &MailMessage{}
	if len(to) == 0 {
		return nil, fmt.Errorf("TO: list cannot be empty")
	}
	mm.To = to
	mm.CC = cc
	mm.BCC = bcc
	mm.Subject = subject
	mm.Body = body
	mm.Attachments = attachments

	return mm, nil
}
