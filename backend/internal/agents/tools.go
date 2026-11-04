package agents

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/pagepilot/backend/internal/services"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// Each tool works only on the requesting user's current document, taken from the turn, never from the model.

type searchArgs struct {
	Query string `json:"query" jsonschema:"what to look up, as a standalone question or keywords"`
}

type excerpt struct {
	Page int    `json:"page"`
	Cite string `json:"cite"` // the exact citation to write, e.g. "[Page 5]"
	Text string `json:"text"`
}

type searchResult struct {
	Excerpts []excerpt `json:"excerpts"`
	Note     string    `json:"note,omitempty"`
}

type quizArgs struct {
	Topic         string   `json:"topic,omitempty" jsonschema:"subject to focus on; empty for the whole document"`
	Difficulty    string   `json:"difficulty,omitempty" jsonschema:"easy, medium or hard"`
	NumQuestions  int      `json:"num_questions,omitempty" jsonschema:"number of questions, 1 to 20; default 5"`
	QuestionTypes []string `json:"question_types,omitempty" jsonschema:"any of mcq, true_false, short_answer; default mcq"`
}

type flashcardArgs struct {
	Topic string `json:"topic,omitempty" jsonschema:"subject to focus on; empty for the whole document"`
	Count int    `json:"count,omitempty" jsonschema:"number of cards, 1 to 30; default 6"`
}

type summaryArgs struct {
	SummaryType string `json:"summary_type,omitempty" jsonschema:"one of complete, chapter, revision, detailed, definitions, concepts, formulas, exam"`
	Topic       string `json:"topic,omitempty" jsonschema:"subject to focus on; empty for the whole document"`
}

type noArgs struct{}

type created struct {
	ID    string `json:"id"`
	Shown string `json:"shown_to_student"`
}

func newTools(svc Services) ([]tool.Tool, error) {
	search, err := functiontool.New(functiontool.Config{
		Name:        "search_document",
		Description: "Searches the student's PDF and returns the most relevant passages with their page numbers.",
	}, func(ctx agent.Context, args searchArgs) (searchResult, error) {
		t, err := turnFrom(ctx)
		if err != nil {
			return searchResult{}, err
		}
		if strings.TrimSpace(args.Query) == "" {
			return searchResult{}, errors.New("query is required")
		}
		results, err := svc.RAG.Retrieve(ctx, args.Query, t.userID, t.documentID, 5)
		if err != nil {
			return searchResult{}, err
		}
		log.Printf("[agent] search_document %q: %d passages", args.Query, len(results))
		t.addRetrieved(results)
		out := searchResult{Excerpts: make([]excerpt, len(results))}
		for i, r := range results {
			out.Excerpts[i] = excerpt{Page: r.PageNumber, Cite: fmt.Sprintf("[Page %d]", r.PageNumber), Text: r.ChunkText}
		}
		if len(results) == 0 {
			out.Note = "No passages found. Tell the student the document doesn't seem to cover this."
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}

	quiz, err := functiontool.New(functiontool.Config{
		Name:        "create_quiz",
		Description: "Creates an interactive quiz from the PDF. The student answers it in the chat.",
	}, func(ctx agent.Context, args quizArgs) (created, error) {
		t, err := turnFrom(ctx)
		if err != nil {
			return created{}, err
		}
		q, err := svc.Quiz.GenerateQuiz(ctx, t.userID, services.GenerateQuizRequest{
			DocumentID: t.documentID, ConversationID: t.conversationID, Topic: args.Topic,
			Difficulty: args.Difficulty, NumQuestions: args.NumQuestions, QuestionTypes: args.QuestionTypes,
		})
		if err != nil {
			return created{}, err
		}
		return finishWith(ctx, t, services.QuizReply(q, args.Topic), q.SessionID), nil
	})
	if err != nil {
		return nil, err
	}

	cards, err := functiontool.New(functiontool.Config{
		Name:        "create_flashcards",
		Description: "Creates a set of question-and-answer flashcards from the PDF.",
	}, func(ctx agent.Context, args flashcardArgs) (created, error) {
		t, err := turnFrom(ctx)
		if err != nil {
			return created{}, err
		}
		set, err := svc.Cards.GenerateFlashcards(ctx, t.userID, services.GenerateFlashcardsRequest{
			DocumentID: t.documentID, ConversationID: t.conversationID, Topic: args.Topic, Count: args.Count,
		})
		if err != nil {
			return created{}, err
		}
		return finishWith(ctx, t, services.FlashcardsReply(set, args.Topic), set.ID), nil
	})
	if err != nil {
		return nil, err
	}

	summary, err := functiontool.New(functiontool.Config{
		Name:        "summarize_document",
		Description: "Writes a summary or study notes for the PDF, with page citations.",
	}, func(ctx agent.Context, args summaryArgs) (created, error) {
		t, err := turnFrom(ctx)
		if err != nil {
			return created{}, err
		}
		sum, err := svc.Summaries.GenerateSummary(ctx, t.userID, services.GenerateSummaryRequest{
			DocumentID: t.documentID, Type: args.SummaryType, Topic: args.Topic,
		})
		if err != nil {
			return created{}, err
		}
		return finishWith(ctx, t, services.SummaryReply(sum), sum.Type), nil
	})
	if err != nil {
		return nil, err
	}

	progress, err := functiontool.New(functiontool.Config{
		Name:        "get_study_progress",
		Description: "Returns the student's quiz scores, weak pages, flashcard progress and suggested next steps for this PDF.",
	}, func(ctx agent.Context, _ noArgs) (*services.ProgressResponse, error) {
		t, err := turnFrom(ctx)
		if err != nil {
			return nil, err
		}
		return svc.Progress.Get(ctx, t.userID, t.documentID)
	})
	if err != nil {
		return nil, err
	}

	return []tool.Tool{search, quiz, cards, summary, progress}, nil
}

// finishWith records a study item as the reply and ends the turn: the item speaks for itself,
// so the model isn't called again just to restate it.
func finishWith(ctx agent.Context, t *turn, reply *services.Reply, id string) created {
	log.Printf("[agent] created %s %s", reply.Type, id)
	t.setStudyItem(reply)
	ctx.Actions().SkipSummarization = true
	return created{ID: id, Shown: fmt.Sprintf("%s shown to the student", reply.Type)}
}
