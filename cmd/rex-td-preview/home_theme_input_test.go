//go:build darwin || linux

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

func TestHomeInputFragmentedThemeAndEscape(t *testing.T) {
	for cut := 1; cut < len(colorSchemeLightNotify); cut++ {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()
			in := &homeInput{File: r}
			if _, err := w.Write(colorSchemeLightNotify[:cut]); err != nil {
				t.Fatal(err)
			}
			go func() {
				time.Sleep(2 * time.Millisecond)
				w.Write(append(bytes.Clone(colorSchemeLightNotify[cut:]), 'j'))
			}()
			var buf [256]byte
			var got []byte
			for len(got) == 0 {
				n, err := in.Read(buf[:])
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, buf[:n]...)
			}
			if string(got) != "j" {
				t.Fatalf("delivered %q to the keyboard handler", got)
			}
		})
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if _, err := w.Write([]byte{27}); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() {
		var buf [256]byte
		n, _ := (&homeInput{File: r}).Read(buf[:])
		done <- string(buf[:n])
	}()
	select {
	case s := <-done:
		if s != "\x1b" {
			t.Fatalf("missing Escape: %q", s)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("Escape waited for another key")
	}
}

func TestHomeInputDelayedReplyAfterIntroducer(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	in := &homeInput{File: r}
	if _, err = w.Write(colorSchemeLightNotify[:4]); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(80 * time.Millisecond)
		w.Write(append(bytes.Clone(colorSchemeLightNotify[4:]), 'v'))
	}()
	var buf [256]byte
	for {
		n, err := in.Read(buf[:])
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			continue
		}
		if string(buf[:n]) != "v" {
			t.Fatalf("leaked a delayed theme report as input: %q", buf[:n])
		}
		break
	}
}

func TestHomeGraphicsReleasesImagesOnAltScreenExit(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	out := &homeGraphics{File: file, baseID: 100}
	out.loaded[codexHomeIcon] = true
	if _, err := io.WriteString(out, "\x1b[?1049l"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte("\x1b_Ga=d,d=I,i=102")) || out.loaded[codexHomeIcon] {
		t.Fatalf("alt screen exit missed the Kitty release: %q", got)
	}
}
