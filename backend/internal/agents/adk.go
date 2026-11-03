package agents

import (
	"context"
	"fmt"
	"log"

	"github.com/pagepilot/backend/internal/config"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/openaimodel"
)

// AgentSystem wraps the Google ADK Go multi-agent coordinator and sub-agents
type AgentSystem struct {
	Coordinator agent.Agent
	Retrieval   agent.Agent
	Tutor       agent.Agent
	Quiz        agent.Agent
	Flashcard   agent.Agent
	Summary     agent.Agent
	Progress    agent.Agent
}

// InitAgentSystem initializes Google ADK agents using Groq or fallback model
func InitAgentSystem(cfg *config.Config) (*AgentSystem, error) {
	ctx := context.Background()

	var llm model.LLM
	var err error

	if cfg.GroqAPIKey != "" {
		llm, err = openaimodel.NewModel(ctx, cfg.GroqModel, &openaimodel.ClientConfig{
			APIKey:  cfg.GroqAPIKey,
			BaseURL: "https://api.groq.com/openai/v1",
			API:     openaimodel.APIChatCompletions,
		})
		if err != nil {
			log.Printf("Warning: Failed to initialize Google ADK openaimodel with Groq: %v. Using standard fallback.", err)
		} else {
			log.Printf("Google ADK Go initialized with model: %s (via Groq OpenAI-compatible endpoint)", cfg.GroqModel)
		}
	}

	// 1. Retrieval Agent
	retrievalAgent, err := llmagent.New(llmagent.Config{
		Name:        "retrieval_agent",
		Description: "Retrieves relevant document chunks and page citations from the vector database.",
		Instruction: "You are the Retrieval Agent. Your job is to extract exact passages, facts, and page numbers from uploaded documents.",
		Model:       llm,
	})
	if err != nil {
		log.Printf("Retrieval agent init warning: %v", err)
	}

	// 2. Tutor Agent
	tutorAgent, err := llmagent.New(llmagent.Config{
		Name:        "tutor_agent",
		Description: "Explains difficult concepts, answers student questions with clear pedagogical steps, and adapts explanation depth.",
		Instruction: "You are the Tutor Agent. Explain concepts clearly, with beginner-friendly analogies, bullet points, and step-by-step reasoning grounded in PDF citations.",
		Model:       llm,
	})
	if err != nil {
		log.Printf("Tutor agent init warning: %v", err)
	}

	// 3. Quiz Agent
	quizAgent, err := llmagent.New(llmagent.Config{
		Name:        "quiz_agent",
		Description: "Generates multiple-choice quiz questions grounded in the PDF and provides immediate answer feedback.",
		Instruction: "You are the Quiz Agent. You formulate thought-provoking questions, validate student choices, and explain why answers are right or wrong with page citations.",
		Model:       llm,
	})
	if err != nil {
		log.Printf("Quiz agent init warning: %v", err)
	}

	// 4. Flashcard Agent
	flashcardAgent, err := llmagent.New(llmagent.Config{
		Name:        "flashcard_agent",
		Description: "Creates high-yield study flashcards with front (prompt/concept) and back (answer/key takeaway).",
		Instruction: "You are the Flashcard Agent. Synthesize dense document topics into active recall flashcards.",
		Model:       llm,
	})
	if err != nil {
		log.Printf("Flashcard agent init warning: %v", err)
	}

	// 5. Summary Agent
	summaryAgent, err := llmagent.New(llmagent.Config{
		Name:        "summary_agent",
		Description: "Produces chapter-wise or full-document structured summaries, revision cheat sheets, and formula lists.",
		Instruction: "You are the Summary Agent. Structure summaries with Executive Summary, Core Concepts, Key Terms, and Page References.",
		Model:       llm,
	})
	if err != nil {
		log.Printf("Summary agent init warning: %v", err)
	}

	// 6. Study Progress Agent
	progressAgent, err := llmagent.New(llmagent.Config{
		Name:        "progress_agent",
		Description: "Analyzes quiz scores, tracks flashcard mastery, identifies weak topics, and recommends next study activities.",
		Instruction: "You are the Study Progress Agent. Track learning velocity and identify knowledge gaps to recommend revision topics.",
		Model:       llm,
	})
	if err != nil {
		log.Printf("Progress agent init warning: %v", err)
	}

	// Build sub-agents list
	var subAgents []agent.Agent
	for _, a := range []agent.Agent{retrievalAgent, tutorAgent, quizAgent, flashcardAgent, summaryAgent, progressAgent} {
		if a != nil {
			subAgents = append(subAgents, a)
		}
	}

	// 7. Coordinator Agent (Root)
	coordinator, err := llmagent.New(llmagent.Config{
		Name:        "coordinator_agent",
		Description: "Root orchestrator that understands user intent and routes tasks to the appropriate specialized study agent.",
		Instruction: `You are the Coordinator Agent for PagePilot.
Analyze user messages to detect intent:
- Concept questions, explanations, follow-ups -> Delegate to tutor_agent
- Quiz requests, tests, knowledge checks -> Delegate to quiz_agent
- Flashcard generation -> Delegate to flashcard_agent
- Summaries, chapter notes, cheat sheets -> Delegate to summary_agent
- Study progress, analytics, weak areas -> Delegate to progress_agent
Ensure all outputs remain grounded in document context.`,
		Model:     llm,
		SubAgents: subAgents,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create coordinator agent: %w", err)
	}

	return &AgentSystem{
		Coordinator: coordinator,
		Retrieval:   retrievalAgent,
		Tutor:       tutorAgent,
		Quiz:        quizAgent,
		Flashcard:   flashcardAgent,
		Summary:     summaryAgent,
		Progress:    progressAgent,
	}, nil
}
