package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"strings"
	"testing"
)

var brand = NewBrand("https://readmely.example/")

func TestNewBrandAcceptsOnlyAWebOrigin(t *testing.T) {
	for in, want := range map[string]string{
		"https://readmely.example/": "https://readmely.example",
		"http://127.0.0.1:3000":     "http://127.0.0.1:3000",
		"javascript:alert(1)":       "",
		"https://x.example/path":    "",
		"":                          "",
	} {
		if got := NewBrand(in).web; got != want {
			t.Errorf("NewBrand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSignInCodeCarriesTheCodeInBothParts(t *testing.T) {
	m := brand.SignInCode("ada@example.invalid", "013842", 10)
	if m.To != "ada@example.invalid" || m.Subject != "Your Readmely sign-in code" {
		t.Fatalf("header: %+v", m)
	}
	if !strings.Contains(m.Text, "013842") || !strings.Contains(m.Text, "10 minutes") {
		t.Fatalf("text: %s", m.Text)
	}
	for _, want := range []string{
		">013842</div>", "In 10 minutes", "https://readmely.example/email/logo.png", "Didn&#39;t ask for this?",
		"ada@example.invalid",
	} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("html lacks %q", want)
		}
	}
}

func TestTheHTMLEscapesWhatItIsGiven(t *testing.T) {
	m := brand.Welcome("ada@example.invalid", `<script>alert("x")</script>`, 100)
	if strings.Contains(m.HTML, "<script>") {
		t.Fatal("a display name reached the HTML unescaped")
	}
	if !strings.Contains(m.HTML, "Welcome aboard, &lt;script&gt;") {
		t.Fatalf("the name is missing: %s", m.HTML[:200])
	}
}

func TestWelcomeHasTheFeaturesCreditsAndTheDevelopersNote(t *testing.T) {
	m := brand.Welcome("ada@example.invalid", "Ada", 100)
	for _, part := range []string{m.Text, m.HTML} {
		for _, want := range []string{"Welcome aboard, Ada!", "100", "Analyse a repository", "Generate a README",
			"Draw diagrams", "Build a context pack", Developer, "Developer of Readmely"} {
			if !strings.Contains(part, want) {
				t.Errorf("lacks %q", want)
			}
		}
	}
	if !strings.Contains(m.HTML, `href="https://readmely.example/dashboard"`) || !strings.Contains(m.Text, "https://readmely.example/dashboard") {
		t.Error("no link to the dashboard")
	}
}

func TestWithoutAWebOriginTheEmailsHaveNoLogoOrLinks(t *testing.T) {
	m := Brand{}.Welcome("ada@example.invalid", "", 100)
	if strings.Contains(m.HTML, "<img") || strings.Contains(m.HTML, "/dashboard") || strings.Contains(m.Text, "/dashboard") {
		t.Fatal("links to the web app without a web origin")
	}
	if !strings.Contains(m.HTML, `href="`+Issues+`"`) {
		t.Fatal("the feedback link needs no web origin and should stay")
	}
	if !strings.Contains(m.HTML, "Welcome aboard!") {
		t.Fatal("no greeting")
	}
}

func TestNoticeSplitsParagraphs(t *testing.T) {
	m := brand.Notice("ada@example.invalid", "Subject", "Security notice", "Heading", "First one.\n\nSecond\none.\n")
	if !strings.Contains(m.HTML, ">First one.</p>") || !strings.Contains(m.HTML, ">Second one.</p>") {
		t.Fatalf("html: %s", m.HTML)
	}
	if m.Text != "First one.\n\nSecond\none.\n" {
		t.Fatalf("text changed: %q", m.Text)
	}
}

func TestComposeSendsTextAndHTMLAsAlternatives(t *testing.T) {
	s, err := NewSMTP(SMTPConfig{Host: "smtp.example", Port: 587, Username: "u", Password: "p",
		From: netmail.Address{Name: "Readmely", Address: "hello@readmely.example"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.compose("ada@example.invalid", brand.SignInCode("ada@example.invalid", "013842", 10))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	kind, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatalf("content type %q: %v", kind, err)
	}
	r := multipart.NewReader(msg.Body, params["boundary"])
	var kinds []string
	for {
		p, err := r.NextPart() // decodes quoted-printable
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(p)
		kinds = append(kinds, strings.SplitN(p.Header.Get("Content-Type"), ";", 2)[0])
		if !bytes.Contains(body, []byte("013842")) {
			t.Errorf("%s part lacks the code", p.Header.Get("Content-Type"))
		}
	}
	if strings.Join(kinds, ",") != "text/plain,text/html" {
		t.Fatalf("parts: %v", kinds)
	}

	plain, err := s.compose("ada@example.invalid", code)
	if err != nil || !bytes.Contains(plain, []byte(`Content-Type: text/plain; charset="utf-8"`)) {
		t.Fatalf("a text-only message changed: %v", err)
	}
}
