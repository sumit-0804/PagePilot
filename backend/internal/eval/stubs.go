package eval

import (
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
)

// stubbedTools generate content; the tools suite only checks which tool was chosen, and the quiz and
// flashcard suites test real generation, so the tools suite skips running them.
var stubbedTools = map[string]bool{"create_quiz": true, "create_flashcards": true, "summarize_document": true}

// StubStudyTools is an agent before-tool callback: returning a result makes ADK skip the tool,
// while its execute_tool span is still recorded.
func StubStudyTools(_ agent.Context, tl tool.Tool, _ map[string]any) (map[string]any, error) {
	if !stubbedTools[tl.Name()] {
		return nil, nil
	}
	return map[string]any{"status": "created", "note": "eval stub: content was not generated"}, nil
}
