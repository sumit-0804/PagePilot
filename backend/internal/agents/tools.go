package agents

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/pagepilot/backend/internal/models"
	"github.com/pagepilot/backend/internal/rag"
	"github.com/pagepilot/backend/internal/services"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// Tools only reach the requesting user's documents attached to this chat, taken from the turn, never from the model.

const searchLimit = 6

type docInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Format   string `json:"format"`
	Sections int    `json:"sections"`
	Unit     string `json:"unit"` // what a section is: Page, Slide or Section
}

type docList struct {
	Documents []docInfo `json:"documents"`
	Note      string    `json:"note,omitempty"`
}

type searchArgs struct {
	Query       string   `json:"query" jsonschema:"what to look up, as a standalone question or keywords"`
	DocumentIDs []string `json:"document_ids,omitempty" jsonschema:"ids of attached documents to search; empty searches all of them"`
}

type excerpt struct {
	Cite     string `json:"cite"` // the exact marker to write, e.g. "[3]"
	Document string `json:"document"`
	Location string `json:"location"` // e.g. "Slide 4"
	Text     string `json:"text"`
}

type searchResult struct {
	Excerpts []excerpt `json:"excerpts"`
	Note     string    `json:"note,omitempty"`
}

type webArgs struct {
	Query string `json:"query" jsonschema:"what to look up on the web, as a standalone query"`
}

type webSource struct {
	Cite  string `json:"cite"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}

type webResult struct {
	Answer  string      `json:"answer"`
	Sources []webSource `json:"sources"`
	Note    string      `json:"note,omitempty"`
}

type quizArgs struct {
	DocumentID    string   `json:"document_id,omitempty" jsonschema:"id of an attached document to quiz on; empty for general knowledge on the topic"`
	Topic         string   `json:"topic,omitempty" jsonschema:"subject to focus on; required without a document"`
	Difficulty    string   `json:"difficulty,omitempty" jsonschema:"easy, medium or hard"`
	NumQuestions  int      `json:"num_questions,omitempty" jsonschema:"number of questions, 1 to 20; default 5"`
	QuestionTypes []string `json:"question_types,omitempty" jsonschema:"any of mcq, true_false, short_answer; default mcq"`
}

type flashcardArgs struct {
	DocumentID string `json:"document_id,omitempty" jsonschema:"id of an attached document; empty for general knowledge on the topic"`
	Topic      string `json:"topic,omitempty" jsonschema:"subject to focus on; required without a document"`
	Count      int    `json:"count,omitempty" jsonschema:"number of cards, 1 to 30; default 6"`
}

type summaryArgs struct {
	DocumentID  string `json:"document_id,omitempty" jsonschema:"id of the attached document to summarize; may be empty when only one is attached"`
	SummaryType string `json:"summary_type,omitempty" jsonschema:"one of complete, chapter, revision, detailed, definitions, concepts, formulas, exam"`
	Topic       string `json:"topic,omitempty" jsonschema:"subject to focus on; empty for the whole document"`
}

type progressArgs struct {
	DocumentID string `json:"document_id,omitempty" jsonschema:"id of the attached document; may be empty when only one is attached"`
}

type noArgs struct{}

type created struct {
	ID    string `json:"id"`
	Shown string `json:"shown_to_user"`
}

// statusFor is what the user sees while a tool runs.
var statusFor = map[string]string{
	"list_documents":     "Checking your documents",
	"search_documents":   "Searching your documents",
	"web_search":         "Searching the web",
	"create_quiz":        "Writing your quiz",
	"create_flashcards":  "Making flashcards",
	"summarize_document": "Summarizing",
	"get_study_progress": "Checking your progress",
}

func newTools(svc Services) ([]tool.Tool, error) {
	var tools []tool.Tool
	add := func(t tool.Tool, err error) error {
		if err == nil {
			tools = append(tools, t)
		}
		return err
	}

	err := errors.Join(
		add(functiontool.New(functiontool.Config{
			Name:        "list_documents",
			Description: "Lists the documents attached to this chat, with their ids.",
		}, func(ctx agent.Context, _ noArgs) (docList, error) {
			t, err := turnFrom(ctx)
			if err != nil {
				return docList{}, err
			}
			out := docList{Documents: make([]docInfo, len(t.docs))}
			for i, d := range t.docs {
				out.Documents[i] = docInfo{ID: d.ID, Name: d.Filename, Format: d.Format, Sections: d.PageCount, Unit: d.SectionLabel()}
			}
			if len(t.docs) == 0 {
				out.Note = "No documents are attached to this chat."
			}
			return out, nil
		})),

		add(functiontool.New(functiontool.Config{
			Name:        "search_documents",
			Description: "Searches the documents attached to this chat and returns the most relevant passages, each with a citation marker.",
		}, func(ctx agent.Context, args searchArgs) (searchResult, error) {
			t, err := turnFrom(ctx)
			if err != nil {
				return searchResult{}, err
			}
			if strings.TrimSpace(args.Query) == "" {
				return searchResult{}, errors.New("query is required")
			}
			ids, err := t.scope(args.DocumentIDs)
			if err != nil {
				return searchResult{}, err
			}
			if len(ids) == 0 {
				return searchResult{Note: "No documents are attached to this chat. Answer from your own knowledge or search the web."}, nil
			}
			results, err := svc.RAG.RetrieveMany(ctx, args.Query, t.userID, ids, searchLimit)
			if err != nil {
				return searchResult{}, err
			}
			log.Printf("[agent] search_documents %q in %d docs: %d passages", args.Query, len(ids), len(results))
			out := searchResult{Excerpts: make([]excerpt, len(results))}
			for i, r := range results {
				label := t.label(r.DocumentID)
				n := t.addSource(models.Citation{
					Kind: models.CitationDocument, DocumentID: r.DocumentID, DocumentName: r.DocumentName,
					PageNumber: r.PageNumber, Label: label, Snippet: rag.Snippet(r.ChunkText, 160),
				})
				out.Excerpts[i] = excerpt{
					Cite: fmt.Sprintf("[%d]", n), Document: r.DocumentName,
					Location: fmt.Sprintf("%s %d", label, r.PageNumber), Text: r.ChunkText,
				}
			}
			if len(results) == 0 {
				out.Note = "No passages found. Say the documents don't seem to cover this."
			}
			return out, nil
		})),

		add(functiontool.New(functiontool.Config{
			Name:        "create_quiz",
			Description: "Creates an interactive quiz from an attached document, or from general knowledge on a topic. The user answers it in the chat.",
		}, func(ctx agent.Context, args quizArgs) (created, error) {
			t, err := turnFrom(ctx)
			if err != nil {
				return created{}, err
			}
			docID, err := t.studyDocument(args.DocumentID, args.Topic)
			if err != nil {
				return created{}, err
			}
			q, err := svc.Quiz.GenerateQuiz(ctx, t.userID, services.GenerateQuizRequest{
				DocumentID: docID, ConversationID: t.conversationID, Topic: args.Topic,
				Difficulty: args.Difficulty, NumQuestions: args.NumQuestions, QuestionTypes: args.QuestionTypes,
			})
			if err != nil {
				return created{}, err
			}
			return finishWith(ctx, t, services.QuizReply(q, args.Topic), q.SessionID), nil
		})),

		add(functiontool.New(functiontool.Config{
			Name:        "create_flashcards",
			Description: "Creates question-and-answer flashcards from an attached document, or from general knowledge on a topic.",
		}, func(ctx agent.Context, args flashcardArgs) (created, error) {
			t, err := turnFrom(ctx)
			if err != nil {
				return created{}, err
			}
			docID, err := t.studyDocument(args.DocumentID, args.Topic)
			if err != nil {
				return created{}, err
			}
			set, err := svc.Cards.GenerateFlashcards(ctx, t.userID, services.GenerateFlashcardsRequest{
				DocumentID: docID, ConversationID: t.conversationID, Topic: args.Topic, Count: args.Count,
			})
			if err != nil {
				return created{}, err
			}
			return finishWith(ctx, t, services.FlashcardsReply(set, args.Topic), set.ID), nil
		})),

		add(functiontool.New(functiontool.Config{
			Name:        "summarize_document",
			Description: "Writes a summary or study notes for an attached document, with citations.",
		}, func(ctx agent.Context, args summaryArgs) (created, error) {
			t, err := turnFrom(ctx)
			if err != nil {
				return created{}, err
			}
			docID, err := t.oneDocument(args.DocumentID)
			if err != nil {
				return created{}, err
			}
			sum, err := svc.Summaries.GenerateSummary(ctx, t.userID, services.GenerateSummaryRequest{
				DocumentID: docID, Type: args.SummaryType, Topic: args.Topic,
			})
			if err != nil {
				return created{}, err
			}
			return finishWith(ctx, t, services.SummaryReply(sum), sum.Type), nil
		})),

		add(functiontool.New(functiontool.Config{
			Name:        "get_study_progress",
			Description: "Returns quiz scores, weak pages, flashcard progress and suggested next steps for an attached document.",
		}, func(ctx agent.Context, args progressArgs) (*services.ProgressResponse, error) {
			t, err := turnFrom(ctx)
			if err != nil {
				return nil, err
			}
			if t.guest {
				return nil, errors.New("progress tracking needs an account; suggest signing up")
			}
			docID, err := t.oneDocument(args.DocumentID)
			if err != nil {
				return nil, err
			}
			return svc.Progress.Get(ctx, t.userID, docID)
		})),
	)
	if err != nil {
		return nil, err
	}

	if svc.Web != nil {
		web, err := functiontool.New(functiontool.Config{
			Name:        "web_search",
			Description: "Searches the live web. Use it for recent events, current facts, or anything you don't know reliably.",
		}, func(ctx agent.Context, args webArgs) (webResult, error) {
			t, err := turnFrom(ctx)
			if err != nil {
				return webResult{}, err
			}
			res, err := svc.Web.Search(ctx, args.Query)
			if err != nil {
				return webResult{}, err
			}
			log.Printf("[agent] web_search %q: %d sources", args.Query, len(res.Sources))
			out := webResult{Answer: res.Answer, Sources: make([]webSource, len(res.Sources))}
			for i, s := range res.Sources {
				n := t.addSource(models.Citation{Kind: models.CitationWeb, URL: s.URL, Title: s.Title, Snippet: rag.Snippet(s.Content, 160)})
				out.Sources[i] = webSource{Cite: fmt.Sprintf("[%d]", n), Title: s.Title, URL: s.URL, Text: s.Content}
			}
			if len(res.Sources) == 0 {
				out.Note = "The search returned no sources; don't cite it."
			}
			return out, nil
		})
		if err != nil {
			return nil, err
		}
		tools = append(tools, web)
	}
	return tools, nil
}

// scope narrows a search to the requested documents, which must be attached to the chat.
func (t *turn) scope(requested []string) ([]string, error) {
	attached := make([]string, len(t.docs))
	for i, d := range t.docs {
		attached[i] = d.ID
	}
	if len(requested) == 0 {
		return attached, nil
	}
	for _, id := range requested {
		if !slices.Contains(attached, id) {
			return nil, fmt.Errorf("document %q is not attached to this chat; call list_documents for valid ids", id)
		}
	}
	return requested, nil
}

// oneDocument resolves the document a tool works on; an empty id is fine when exactly one is attached.
func (t *turn) oneDocument(id string) (string, error) {
	switch {
	case id != "":
		if _, err := t.scope([]string{id}); err != nil {
			return "", err
		}
		return id, nil
	case len(t.docs) == 1:
		return t.docs[0].ID, nil
	case len(t.docs) == 0:
		return "", errors.New("no documents are attached to this chat")
	default:
		return "", errors.New("several documents are attached; pass document_id (see list_documents)")
	}
}

// studyDocument picks the source of a quiz or flashcards: a named document, the only attached
// one when no topic is given, or none (general knowledge) for a topic.
func (t *turn) studyDocument(id, topic string) (string, error) {
	if id != "" || (strings.TrimSpace(topic) == "" && len(t.docs) > 0) {
		return t.oneDocument(id)
	}
	if strings.TrimSpace(topic) == "" {
		return "", errors.New("give a topic, or attach a document to study from")
	}
	return "", nil
}

func (t *turn) label(docID string) string {
	for _, d := range t.docs {
		if d.ID == docID {
			return d.SectionLabel()
		}
	}
	return "Page"
}

// finishWith records a study item as the reply and ends the turn: the item speaks for itself,
// so the model isn't called again just to restate it.
func finishWith(ctx agent.Context, t *turn, reply *services.Reply, id string) created {
	log.Printf("[agent] created %s %s", reply.Type, id)
	t.setStudyItem(reply)
	ctx.Actions().SkipSummarization = true
	return created{ID: id, Shown: fmt.Sprintf("%s shown to the user", reply.Type)}
}
