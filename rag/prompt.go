package main

import (
	"fmt"
	"strings"
)

// answerSystemPrompt keeps the model inside the retrieved material. The whole
// point of this service is that every claim traces back to an uploaded
// document, so refusing to answer beats filling gaps from memory.
const answerSystemPrompt = `Jesteś asystentem, który odpowiada WYŁĄCZNIE na podstawie dostarczonych fragmentów źródeł.

Zasady:
1. Korzystaj tylko z treści fragmentów. Nie dopowiadaj niczego z własnej wiedzy.
2. Po każdym zdaniu opartym na źródle podaj odnośnik w nawiasie kwadratowym, np. [1] albo [2][3].
3. Jeśli fragmenty nie zawierają odpowiedzi, napisz dokładnie: "Nie znalazłem odpowiedzi w dostępnych materiałach." i nic więcej.
4. Odpowiadaj po polsku, zwięźle i konkretnie.
5. Nie cytuj numeru źródła, którego nie ma na liście poniżej.`

// buildUserPrompt lays the citations out for the model, numbered exactly the
// way the Source.Ref values are, so the [n] markers it emits line up with what
// the caller gets back.
func buildUserPrompt(question string, sources []Source) string {
	var sb strings.Builder

	sb.WriteString("FRAGMENTY ŹRÓDEŁ\n\n")
	for _, source := range sources {
		fmt.Fprintf(&sb, "[%d] %s", source.Ref, source.SourceKey)
		if source.Timecode != "" {
			fmt.Fprintf(&sb, " (%s)", source.Timecode)
		}
		sb.WriteString("\n")
		sb.WriteString(source.Quote)
		sb.WriteString("\n\n")
	}

	sb.WriteString("PYTANIE\n")
	sb.WriteString(question)

	return sb.String()
}
