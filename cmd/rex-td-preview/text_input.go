package main

import (
	"slices"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type lineInput struct {
	value  string
	cursor int // Byte offset at a grapheme boundary.
}

func (input *lineInput) set(value string) {
	input.value, input.cursor = value, len(value)
}

func (input *lineInput) update(msg tea.KeyMsg) bool {
	previous := input.value
	switch msg.Type {
	case tea.KeyHome, tea.KeyCtrlA:
		input.cursor = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		input.cursor = len(input.value)
	case tea.KeyCtrlU:
		input.set("")
	case tea.KeyLeft, tea.KeyRight, tea.KeyBackspace, tea.KeyCtrlH, tea.KeyDelete, tea.KeyCtrlD:
		edges := graphemeEdges(input.value)
		at, _ := slices.BinarySearch(edges, input.cursor)
		left, right := edges[max(0, at-1)], edges[min(at+1, len(edges)-1)]
		switch msg.Type {
		case tea.KeyLeft:
			input.cursor = left
		case tea.KeyRight:
			input.cursor = right
		case tea.KeyBackspace, tea.KeyCtrlH:
			input.replace(left, input.cursor, "")
		case tea.KeyDelete, tea.KeyCtrlD:
			input.replace(input.cursor, right, "")
		}
	case tea.KeyRunes, tea.KeySpace:
		runes := msg.Runes
		if msg.Type == tea.KeySpace {
			runes = []rune{' '}
		}
		remaining := 240 - utf8.RuneCountInString(input.value)
		value := storedDisplay(string(runes))
		end := 0
		for rest, state := value, -1; rest != "" && remaining > 0; {
			cluster, next, _, nextState := ansi.FirstGraphemeCluster(rest, state)
			count := utf8.RuneCountInString(cluster)
			if count > remaining {
				break
			}
			end += len(cluster)
			remaining -= count
			rest, state = next, nextState
		}
		if end != 0 {
			input.replace(input.cursor, input.cursor, value[:end])
		}
	}
	return input.value != previous
}

func (input *lineInput) replace(start, end int, text string) {
	if start == end && text == "" {
		return
	}
	input.value = input.value[:start] + text + input.value[end:]
	// Inserting or removing text can join graphemes across the edit.
	edges := graphemeEdges(input.value)
	at, _ := slices.BinarySearch(edges, start+len(text))
	input.cursor = edges[at]
}

func graphemeEdges(value string) []int {
	edges := []int{0}
	offset := 0
	for rest, state := value, -1; rest != ""; {
		cluster, next, _, nextState := ansi.FirstGraphemeCluster(rest, state)
		offset += len(cluster)
		edges = append(edges, offset)
		rest, state = next, nextState
	}
	return edges
}

func (input lineInput) view(width int) string {
	const cursor = "▏"
	available := width - ansi.StringWidth(cursor)
	if available < 0 {
		return ""
	}
	left, right := input.value[:input.cursor], input.value[input.cursor:]
	leftWidth, rightWidth := ansi.StringWidth(left), ansi.StringWidth(right)
	if leftWidth+rightWidth <= available {
		return left + cursor + right
	}
	leftBudget := available - min(rightWidth, available/3)
	state := -1
	for leftWidth > leftBudget && left != "" {
		_, rest, width, nextState := ansi.FirstGraphemeCluster(left, state)
		left, state = rest, nextState
		leftWidth -= width
	}
	right = ansi.Truncate(right, available-leftWidth, "")
	return left + cursor + right
}
