package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (e *Engine) evalDefine(ctx context.Context, body string) []Result {
	word := strings.TrimSpace(body)
	if word == "" || strings.ContainsAny(word, " \t") {
		return nil
	}
	url := e.dictURL(word)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := e.http().Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil
	}
	title, sub, ok := parseDictionary(raw)
	if !ok {
		return nil
	}
	return []Result{{
		Kind:     KindDefine,
		Title:    title,
		Subtitle: sub,
		Copy:     title,
		Rank:     40,
	}}
}

func (e *Engine) dictURL(word string) string {
	if e.DictURL != nil {
		return e.DictURL(word)
	}
	return "https://api.dictionaryapi.dev/api/v2/entries/en/" + word
}

func parseDictionary(raw []byte) (title, sub string, ok bool) {
	var parsed []struct {
		Word     string `json:"word"`
		Meanings []struct {
			PartOfSpeech string `json:"partOfSpeech"`
			Definitions  []struct {
				Definition string `json:"definition"`
			} `json:"definitions"`
		} `json:"meanings"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed) == 0 {
		return "", "", false
	}
	e := parsed[0]
	if len(e.Meanings) == 0 || len(e.Meanings[0].Definitions) == 0 {
		return "", "", false
	}
	def := strings.TrimSpace(e.Meanings[0].Definitions[0].Definition)
	if def == "" {
		return "", "", false
	}
	word := e.Word
	if word == "" {
		word = "—"
	}
	pos := e.Meanings[0].PartOfSpeech
	title = fmt.Sprintf("%s — %s", word, def)
	return title, pos, true
}
