package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"strings"
)

// Brand builds the product's emails: a plain-text body for every client and the log, and an HTML
// body in one shared layout. Every value put into the HTML is escaped by html/template, and the
// layout is tables with inline styles, which is what email clients reliably draw.
type Brand struct {
	web  string // the web app's origin, like https://readmely.gitanshu.me; empty leaves links out
	host string
}

// NewBrand takes the web app's origin, used for the logo and links. Anything but an http(s) origin
// is ignored and the emails go out without them.
func NewBrand(webOrigin string) Brand {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(webOrigin), "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != "" {
		return Brand{}
	}
	return Brand{web: u.Scheme + "://" + u.Host, host: u.Host}
}

// Developer signs the welcome note, and Issues is where people can reach him.
const (
	Developer = "Gitanshu Sankhla"
	Issues    = "https://github.com/Gitax18/readmely/issues"
)

const (
	font = `-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif`
	mono = `'SFMono-Regular',Menlo,Consolas,'Liberation Mono',monospace`
)

// The font stacks are spliced into the template source before parsing, so they need no escaping
// bypass at run time.
var layouts = template.Must(template.New("mail").Funcs(template.FuncMap{
	"inc": func(i int) int { return i + 1 },
}).Parse(strings.NewReplacer("{{font}}", font, "{{mono}}", mono).Replace(`
{{define "top"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light">
<meta name="supported-color-schemes" content="light">
<title>{{.Subject}}</title>
</head>
<body style="margin:0;padding:0;background-color:#eef3f4;-webkit-text-size-adjust:100%;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;font-size:1px;line-height:1px;">{{.Preheader}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#eef3f4;">
<tr><td align="center" style="padding:32px 14px 40px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;">
<tr><td style="padding:0 6px 18px;">
<table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr>
{{if .Web}}<td style="vertical-align:middle;padding-right:10px;"><a href="{{.Web}}"><img src="{{.Web}}/email/logo.png" width="38" height="38" alt="Readmely" style="display:block;border:0;border-radius:10px;"></a></td>{{end}}
<td style="vertical-align:middle;font-family:{{font}};font-size:20px;font-weight:700;letter-spacing:-0.4px;color:#142126;">Readmely</td>
</tr></table>
</td></tr>
<tr><td style="background-color:#ffffff;border:1px solid #dde6e8;border-radius:16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="height:5px;line-height:5px;font-size:0;background-color:#087c72;background-image:linear-gradient(90deg,#087c72,#1ba99b);border-radius:16px 16px 0 0;">&nbsp;</td></tr>
<tr><td style="padding:34px 34px 30px;font-family:{{font}};color:#142126;">
<p style="margin:0 0 10px;font-size:12px;font-weight:700;letter-spacing:1.4px;text-transform:uppercase;color:#087c72;">{{.Eyebrow}}</p>
<h1 style="margin:0 0 14px;font-size:26px;line-height:32px;font-weight:700;letter-spacing:-0.6px;color:#142126;">{{.Heading}}</h1>
{{end}}

{{define "bottom"}}</td></tr>
</table>
</td></tr>
<tr><td style="padding:22px 12px 0;font-family:{{font}};font-size:12px;line-height:19px;color:#6b7f84;text-align:center;">
{{.Footer}}<br>
{{if .Web}}<a href="{{.Web}}" style="color:#087c72;text-decoration:none;font-weight:600;">{{.Host}}</a>
&nbsp;·&nbsp; <a href="{{.Web}}/privacy" style="color:#6b7f84;">Privacy</a>
&nbsp;·&nbsp; <a href="{{.Web}}/terms" style="color:#6b7f84;">Terms</a>{{else}}Readmely{{end}}
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>{{end}}

{{define "paragraphs"}}{{range .}}<p style="margin:0 0 14px;font-size:15px;line-height:24px;color:#34474c;">{{.}}</p>{{end}}{{end}}

{{define "button"}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:6px 0 4px;"><tr>
<td style="border-radius:10px;background-color:#087c72;"><a href="{{.URL}}" style="display:inline-block;padding:13px 24px;font-family:{{font}};font-size:15px;font-weight:700;color:#ffffff;text-decoration:none;border-radius:10px;">{{.Label}} &rarr;</a></td>
</tr></table>{{end}}

{{define "code"}}{{template "top" .}}
{{template "paragraphs" .Intro}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:8px 0 22px;">
<tr><td align="center" style="padding:24px 12px;background-color:#f0f8f7;border:1px dashed #8fcbc3;border-radius:14px;">
<div style="font-family:{{mono}};font-size:38px;line-height:44px;font-weight:700;letter-spacing:12px;color:#0b4f49;padding-left:12px;">{{.Code}}</div>
<div style="margin-top:8px;font-family:{{font}};font-size:12px;color:#5b7377;">Enter or paste it on the page you came from</div>
</td></tr>
</table>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 22px;font-size:14px;">
{{range .Details}}<tr>
<td style="padding:10px 0;border-top:1px solid #e8eef0;color:#6b7f84;width:38%;">{{.Label}}</td>
<td style="padding:10px 0;border-top:1px solid #e8eef0;color:#142126;font-weight:600;">{{.Value}}</td>
</tr>{{end}}
</table>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
<tr><td style="padding:14px 16px;background-color:{{if .Warn}}#fdf3f1{{else}}#f6f8f9{{end}};border-left:3px solid {{if .Warn}}#c2410c{{else}}#9fb3b8{{end}};border-radius:8px;font-size:13px;line-height:20px;color:#34474c;">
<strong style="color:#142126;">{{.NoteTitle}}</strong><br>{{.Note}}
</td></tr>
</table>
{{template "bottom" .}}{{end}}

{{define "notice"}}{{template "top" .}}
{{template "paragraphs" .Intro}}
{{if .Button.URL}}{{template "button" .Button}}{{end}}
{{template "bottom" .}}{{end}}

{{define "welcome"}}{{template "top" .}}
{{template "paragraphs" .Intro}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin:4px 0 24px;">
<tr><td style="padding:16px 18px;background-color:#e6f4f1;border-radius:12px;">
<span style="font-size:28px;font-weight:800;letter-spacing:-0.8px;color:#087c72;">{{.Credits}}</span>
<span style="font-size:14px;color:#0b4f49;font-weight:600;">&nbsp;free credits are in your account</span><br>
<span style="font-size:13px;line-height:20px;color:#3d5a5e;">Each job holds the most it could cost and you are charged only what it used. A job that fails is refunded in full.</span>
</td></tr>
</table>
{{if .Button.URL}}{{template "button" .Button}}{{end}}
<h2 style="margin:30px 0 6px;font-size:17px;font-weight:700;color:#142126;">What to explore first</h2>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
{{range $i, $f := .Features}}<tr>
<td style="vertical-align:top;padding:14px 14px 14px 0;width:34px;">
<div style="width:30px;height:30px;line-height:30px;border-radius:15px;background-color:#132b30;color:#ffffff;font-size:13px;font-weight:700;text-align:center;">{{inc $i}}</div>
</td>
<td style="vertical-align:top;padding:12px 0;border-bottom:1px solid #eef2f3;">
<div style="font-size:15px;font-weight:700;color:#142126;margin-bottom:3px;">{{$f.Title}}</div>
<div style="font-size:14px;line-height:21px;color:#4b5f64;">{{$f.Text}}</div>
</td>
</tr>{{end}}
</table>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin-top:28px;">
<tr><td style="padding:22px 22px 20px;background-color:#f7f9fa;border:1px solid #e4ebed;border-radius:14px;">
<p style="margin:0 0 10px;font-size:12px;font-weight:700;letter-spacing:1.2px;text-transform:uppercase;color:#6b7f84;">A note from the developer</p>
{{range .Letter}}<p style="margin:0 0 12px;font-size:15px;line-height:24px;color:#34474c;font-style:italic;">{{.}}</p>{{end}}
<p style="margin:0 0 4px;font-size:14px;"><a href="{{.IssuesURL}}" style="color:#087c72;font-weight:600;text-decoration:none;">Share feedback on GitHub &rarr;</a></p>
<p style="margin:14px 0 0;font-size:15px;line-height:20px;color:#142126;"><strong>{{.Signer}}</strong><br><span style="font-size:13px;color:#6b7f84;">Developer of Readmely</span></p>
</td></tr>
</table>
{{template "bottom" .}}{{end}}
`)))

type detail struct{ Label, Value string }

type button struct{ Label, URL string }

type feature struct{ Title, Text string }

// page is everything the layout reads. Fields a template does not use stay empty.
type page struct {
	Subject, Preheader, Eyebrow, Heading, Footer string
	Web, Host                                    string
	Intro                                        []string
	Button                                       button

	// the code email
	Code            string
	Details         []detail
	NoteTitle, Note string
	Warn            bool

	// the welcome email
	Credits   int64
	Features  []feature
	Letter    []string
	Signer    string
	IssuesURL string
}

func (b Brand) render(name string, p page) string {
	p.Web, p.Host = b.web, b.host
	var out bytes.Buffer
	if err := layouts.ExecuteTemplate(&out, name, p); err != nil {
		// The templates are fixed and the data is plain strings: this cannot fail at run time, and
		// the plain-text part still carries everything if it somehow did.
		return ""
	}
	return out.String()
}

func (b Brand) link(path string) string {
	if b.web == "" {
		return ""
	}
	return b.web + path
}

// SignInCode is the email that carries a sign-in code.
func (b Brand) SignInCode(to, code string, minutes int) Message {
	subject := "Your Readmely sign-in code"
	text := fmt.Sprintf("Your Readmely sign-in code is %s\n\n"+
		"Enter it on the sign-in page to continue. It works once and expires in %d minutes.\n\n"+
		"Didn't ask for it? You can ignore this email: nobody can sign in without the code. "+
		"Readmely will never ask you to share it.\n", code, minutes)
	html := b.render("code", page{
		Subject:   subject,
		Preheader: fmt.Sprintf("%s is your code. It expires in %d minutes.", code, minutes),
		Eyebrow:   "Sign-in code",
		Heading:   "Here is your code",
		Intro:     []string{"Use this code to finish signing in to Readmely. If you have no account yet, it creates one."},
		Code:      code,
		Details: []detail{
			{"Expires", fmt.Sprintf("In %d minutes", minutes)},
			{"Works", "Once, then it is spent"},
			{"Sent to", to},
		},
		NoteTitle: "Didn't ask for this?",
		Note:      "You can ignore this email: nobody can sign in without the code. Readmely will never ask you to share it.",
		Footer:    "You received this because a sign-in to Readmely was requested for " + to + ".",
	})
	return Message{To: to, Subject: subject, Text: text, HTML: html}
}

// DeletionCode is the email that carries the code confirming an account deletion.
func (b Brand) DeletionCode(to, code string, minutes int) Message {
	subject := "Confirm deleting your Readmely account"
	text := fmt.Sprintf("Your code to delete your Readmely account is %s\n\nIt works once and expires in %d minutes. "+
		"Deleting the account removes your projects and everything generated for them, and cannot be undone. "+
		"If you did not ask for this, do not share the code: someone may be using your session, so sign in again "+
		"and sign out of any session you do not recognise under Account.\n", code, minutes)
	html := b.render("code", page{
		Subject:   subject,
		Preheader: fmt.Sprintf("Your code to delete your account. It expires in %d minutes.", minutes),
		Eyebrow:   "Account deletion",
		Heading:   "Confirm deleting your account",
		Intro: []string{"Enter this code to delete your Readmely account. Your projects, READMEs, diagrams and " +
			"credit history are removed for good, and this cannot be undone."},
		Code: code,
		Details: []detail{
			{"Expires", fmt.Sprintf("In %d minutes", minutes)},
			{"Works", "Once, then it is spent"},
			{"Account", to},
		},
		Warn:      true,
		NoteTitle: "Didn't ask for this?",
		Note: "Do not share the code. Someone may be using your session: sign in again and sign out of any " +
			"session you do not recognise under Account.",
		Footer: "You received this because deleting the Readmely account for " + to + " was requested.",
	})
	return Message{To: to, Subject: subject, Text: text, HTML: html}
}

// Notice is a short message in the shared layout: security alerts and account changes. Its text is
// split into paragraphs on blank lines for the HTML part.
func (b Brand) Notice(to, subject, eyebrow, heading, text string) Message {
	var paras []string
	for _, p := range strings.Split(strings.TrimSpace(text), "\n\n") {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			paras = append(paras, p)
		}
	}
	html := b.render("notice", page{
		Subject:   subject,
		Preheader: firstSentence(text),
		Eyebrow:   eyebrow,
		Heading:   heading,
		Intro:     paras,
		Button:    button{Label: "Review your account", URL: b.link("/account")},
		Footer:    "This is a notice about the Readmely account for " + to + ".",
	})
	return Message{To: to, Subject: subject, Text: text, HTML: html}
}

// Welcome greets a new account: what it can do first, the free credits, and a note from the developer.
func (b Brand) Welcome(to, name string, credits int64) Message {
	subject := "Welcome to Readmely"
	hello := "Welcome aboard!"
	if name = strings.TrimSpace(name); name != "" {
		hello = "Welcome aboard, " + name + "!"
	}
	features := []feature{
		{"Analyse a repository", "Paste a GitHub link. Readmely reads the code and maps its languages, frameworks, " +
			"entry points, commands and settings, without keeping any secret it finds."},
		{"Generate a README", "Badges, tables, setup steps, a file tree and an architecture diagram, in three " +
			"layouts. Edit it in place, or have any one section rewritten."},
		{"Draw diagrams", "Architecture and database diagrams drawn from the real files, ready to download as SVG or PNG."},
		{"Build a context pack", "Files that AI coding tools can read to understand the project. Always free."},
	}
	note := []string{
		"Thank you for signing up. I built Readmely because a good README is the first thing people see of a " +
			"project, and usually the last thing anyone has time to write. I hope it gives you that time back.",
		"If something breaks, or you have an idea that would make it better, I would love to hear about it: " +
			"open an issue on GitHub and I will read it.",
	}
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\nYour Readmely account is ready, with %d free credits to try everything.\n", hello, credits)
	if b.web != "" {
		fmt.Fprintf(&text, "Open your dashboard: %s/dashboard\n", b.web)
	}
	text.WriteString("\nWhat to explore first\n")
	for i, f := range features {
		fmt.Fprintf(&text, "\n%d. %s\n   %s\n", i+1, f.Title, f.Text)
	}
	text.WriteString("\nA note from the developer\n\n")
	for _, n := range note {
		text.WriteString(n + "\n\n")
	}
	text.WriteString(Issues + "\n\n")
	fmt.Fprintf(&text, "%s\nDeveloper of Readmely\n", Developer)

	html := b.render("welcome", page{
		Subject:   subject,
		Preheader: fmt.Sprintf("Your account is ready, with %d free credits. Here is where to start.", credits),
		Eyebrow:   "Welcome",
		Heading:   hello,
		Intro: []string{"Your account is ready. Readmely turns a GitHub repository into a polished README, " +
			"clear diagrams and context files for AI tools, in a few minutes."},
		Credits:   credits,
		Button:    button{Label: "Open your dashboard", URL: b.link("/dashboard")},
		Features:  features,
		Letter:    note,
		IssuesURL: Issues,
		Signer:    Developer,
		Footer:    "You received this because a Readmely account was created for " + to + ".",
	})
	return Message{To: to, Subject: subject, Text: text.String(), HTML: html}
}

func firstSentence(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	if i := strings.Index(t, ". "); i > 0 {
		t = t[:i+1]
	}
	if len(t) > 140 {
		t = t[:140]
	}
	return t
}
