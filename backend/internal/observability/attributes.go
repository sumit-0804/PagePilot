package observability

import "go.opentelemetry.io/otel/attribute"

// Langfuse reads these from OTLP spans: https://langfuse.com/integrations/native/opentelemetry
const (
	AttrTraceName       = attribute.Key("langfuse.trace.name")
	AttrTraceTags       = attribute.Key("langfuse.trace.tags")
	AttrUserID          = attribute.Key("langfuse.user.id")
	AttrSessionID       = attribute.Key("langfuse.session.id")
	AttrObservationType = attribute.Key("langfuse.observation.type")
	AttrLevel           = attribute.Key("langfuse.observation.level")
	AttrStatusMessage   = attribute.Key("langfuse.observation.status_message")
	AttrInput           = attribute.Key("langfuse.observation.input")
	AttrOutput          = attribute.Key("langfuse.observation.output")
	AttrEnvironment     = attribute.Key("langfuse.environment")
	AttrRelease         = attribute.Key("langfuse.release")
)

// OpenTelemetry GenAI semantic conventions, which Langfuse also maps.
const (
	AttrGenAIOperation    = attribute.Key("gen_ai.operation.name")
	AttrGenAISystem       = attribute.Key("gen_ai.provider.name")
	AttrGenAIRequestModel = attribute.Key("gen_ai.request.model")
	AttrGenAIInputTokens  = attribute.Key("gen_ai.usage.input_tokens")
	AttrGenAIOutputTokens = attribute.Key("gen_ai.usage.output_tokens")
)

// PagePilot-specific attributes. Metadata under langfuse.observation.metadata.* is filterable in Langfuse.
const (
	AttrUsageAvailable  = attribute.Key("langfuse.observation.metadata.usage_available")
	AttrGuest           = attribute.Key("langfuse.observation.metadata.guest")
	AttrDocumentCount   = attribute.Key("langfuse.observation.metadata.document_count")
	AttrDocumentID      = attribute.Key("langfuse.observation.metadata.document_id")
	AttrDocumentFormat  = attribute.Key("langfuse.observation.metadata.document_format")
	AttrReplyType       = attribute.Key("langfuse.observation.metadata.reply_type")
	AttrCitationCount   = attribute.Key("langfuse.observation.metadata.citation_count")
	AttrTopK            = attribute.Key("langfuse.observation.metadata.top_k")
	AttrQueryChars      = attribute.Key("langfuse.observation.metadata.query_chars")
	AttrChunksReturned  = attribute.Key("langfuse.observation.metadata.chunks_returned")
	AttrTopScore        = attribute.Key("langfuse.observation.metadata.top_score")
	AttrMinScore        = attribute.Key("langfuse.observation.metadata.min_score")
	AttrBatchSize       = attribute.Key("langfuse.observation.metadata.batch_size")
	AttrPageCount       = attribute.Key("langfuse.observation.metadata.page_count")
	AttrChunkCount      = attribute.Key("langfuse.observation.metadata.chunk_count")
	AttrItemCount       = attribute.Key("langfuse.observation.metadata.item_count")
	AttrStorageBytes    = attribute.Key("langfuse.observation.metadata.bytes")
	AttrResponseFormat  = attribute.Key("langfuse.observation.metadata.response_format")
	AttrStreaming       = attribute.Key("langfuse.observation.metadata.streaming")
	AttrHTTPMethod      = attribute.Key("http.request.method")
	AttrHTTPRoute       = attribute.Key("http.route")
	AttrHTTPStatusCode  = attribute.Key("http.response.status_code")
	AttrHTTPRequestSize = attribute.Key("http.request.body.size")
)
