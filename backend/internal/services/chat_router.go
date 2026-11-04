package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/pagepilot/backend/internal/rag"
)

const (
	intentQuestion   = "question"
	intentQuiz       = "quiz"
	intentFlashcards = "flashcards"
	intentSummary    = "summary"
)

// route is the router's reading of one chat message.
type route struct {
	Intent        string   `json:"intent"`
	SearchQuery   string   `json:"search_query"`
	Style         string   `json:"style"`
	Topic         string   `json:"topic"`
	Difficulty    string   `json:"difficulty"`
	NumQuestions  int      `json:"num_questions"`
	QuestionTypes []string `json:"question_types"`
	Count         int      `json:"count"`
	SummaryType   string   `json:"summary_type"`
}

const routerSystemPrompt = "You are the intent router of a study assistant. Respond ONLY with a JSON object."

// routeMessage asks the LLM what the user wants and rewrites follow-ups into a standalone search query.
func routeMessage(ctx context.Context, llm rag.LLMClient, history []rag.ChatMessage, message string) (route, error) {
	var recent strings.Builder
	for _, m := range history[max(0, len(history)-6):] {
		fmt.Fprintf(&recent, "%s: %s\n", m.Role, rag.Snippet(m.Content, 400))
	}
	if recent.Len() == 0 {
		recent.WriteString("(none)\n")
	}

	prompt := fmt.Sprintf(`A student is chatting with a study assistant about one PDF.
Decide what the latest message asks for.

intent:
- "quiz": they want to be quizzed or tested ("test my understanding", "quiz me", "give me questions")
- "flashcards": they want flashcards
- "summary": they want a summary, notes, revision notes, definitions, key concepts, formulas or exam notes for the document
- "question": anything else (questions, explanations, follow-ups)

search_query: for "question", rewrite the message as a standalone question with the subject spelled out, using the conversation (e.g. "explain it more simply" after a talk about neural networks becomes "What are neural networks?"). Otherwise "".
style: if they ask for a way of explaining, one of %s; else "".
topic: the subject a quiz, flashcards or summary should focus on, taken from the message or the conversation; "" for the whole document.
difficulty: "easy", "medium" or "hard" if they say so; else "".
num_questions: number of quiz questions if they say so; else 0.
question_types: any of "mcq", "true_false", "short_answer" if they say so; else [].
count: number of flashcards if they say so; else 0.
summary_type: one of %s if they want a summary; else "".

CONVERSATION SO FAR:
%s
LATEST MESSAGE: %s

Respond with JSON: {"intent": "...", "search_query": "...", "style": "", "topic": "", "difficulty": "", "num_questions": 0, "question_types": [], "count": 0, "summary_type": ""}`,
		quoteKeys(rag.ExplanationStyles), quoteKeys(summaryInstructions), recent.String(), message)

	resp, err := llm.GenerateJSON(ctx, []rag.ChatMessage{
		{Role: "system", Content: routerSystemPrompt},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return route{}, err
	}
	var r route
	if err := json.Unmarshal([]byte(resp), &r); err != nil {
		return route{}, fmt.Errorf("parsing router output: %w", err)
	}

	// Ignore anything outside the allowed values rather than trusting the model.
	if !slices.Contains([]string{intentQuestion, intentQuiz, intentFlashcards, intentSummary}, r.Intent) {
		r.Intent = intentQuestion
	}
	if _, ok := rag.ExplanationStyles[r.Style]; !ok {
		r.Style = ""
	}
	if _, ok := summaryInstructions[r.SummaryType]; !ok {
		r.SummaryType = ""
	}
	r.QuestionTypes = slices.DeleteFunc(r.QuestionTypes, func(t string) bool { return !slices.Contains(questionTypes, t) })
	r.Topic = rag.Snippet(strings.TrimSpace(r.Topic), 200)
	return r, nil
}

// DirectResponder answers with one router call plus the matching service, without an agent loop.
// It is the CHAT_ENGINE=direct fallback for the ADK coordinator.
type DirectResponder struct {
	llm       rag.LLMClient
	ragEngine *rag.Engine
	quiz      *QuizService
	cards     *FlashcardService
	summaries *SummaryService
}

func NewDirectResponder(llm rag.LLMClient, ragEngine *rag.Engine, quiz *QuizService, cards *FlashcardService, summaries *SummaryService) *DirectResponder {
	return &DirectResponder{llm: llm, ragEngine: ragEngine, quiz: quiz, cards: cards, summaries: summaries}
}

func (d *DirectResponder) Respond(ctx context.Context, req ReplyRequest, streamWriter func(string) error) (*Reply, error) {
	conv := req.Conversation
	r, err := routeMessage(ctx, d.llm, req.History, req.Message)
	if err != nil {
		// Routing only adds intent detection and query rewriting; answering the message directly still works.
		log.Printf("chat router failed, answering as a question: %v", err)
		r = route{Intent: intentQuestion}
	}

	var reply *Reply
	switch r.Intent {
	case intentQuiz:
		quiz, err := d.quiz.GenerateQuiz(ctx, req.UserID, GenerateQuizRequest{
			DocumentID: conv.DocumentID, ConversationID: conv.ID, Topic: r.Topic,
			Difficulty: r.Difficulty, NumQuestions: r.NumQuestions, QuestionTypes: r.QuestionTypes,
		})
		if err != nil {
			return nil, err
		}
		reply = QuizReply(quiz, r.Topic)
	case intentFlashcards:
		set, err := d.cards.GenerateFlashcards(ctx, req.UserID, GenerateFlashcardsRequest{
			DocumentID: conv.DocumentID, ConversationID: conv.ID, Topic: r.Topic, Count: r.Count,
		})
		if err != nil {
			return nil, err
		}
		reply = FlashcardsReply(set, r.Topic)
	case intentSummary:
		sum, err := d.summaries.GenerateSummary(ctx, req.UserID, GenerateSummaryRequest{DocumentID: conv.DocumentID, Type: r.SummaryType, Topic: r.Topic})
		if err != nil {
			return nil, err
		}
		reply = SummaryReply(sum)
	}
	if reply != nil {
		// Study items arrive all at once; send their text as a single token so the UI stays uniform.
		if streamWriter != nil {
			if err := streamWriter(reply.Content); err != nil {
				return nil, err
			}
		}
		return reply, nil
	}

	style := req.Style
	if style == "" {
		style = r.Style
	}
	params := rag.AskParams{
		UserID: req.UserID, DocumentID: conv.DocumentID, Question: req.Message,
		SearchQuery: r.SearchQuery, Style: style, History: req.History,
	}
	var result *rag.RAGResult
	if streamWriter != nil {
		result, err = d.ragEngine.StreamQuery(ctx, params, streamWriter)
	} else {
		result, err = d.ragEngine.Query(ctx, params)
	}
	if err != nil {
		return nil, fmt.Errorf("generating answer: %w", err)
	}
	return &Reply{Content: result.Answer, Type: "chat", Citations: result.Citations}, nil
}

func quoteKeys[V any](m map[string]V) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, fmt.Sprintf("%q", k))
	}
	slices.Sort(keys)
	return strings.Join(keys, ", ")
}
