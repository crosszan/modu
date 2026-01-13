// Package rpc implements the Google batchexecute RPC protocol
package rpc

const (
	// BatchExecuteURL is the main RPC endpoint
	BatchExecuteURL = "https://notebooklm.google.com/_/LabsTailwindUi/data/batchexecute"

	// QueryURL is the streaming endpoint for chat
	QueryURL = "https://notebooklm.google.com/_/LabsTailwindUi/data/google.internal.labs.tailwind.orchestration.v1.LabsTailwindOrchestrationService/GenerateFreeFormStreamed"

	// UploadURL is for file uploads
	UploadURL = "https://notebooklm.google.com/upload/_/"

	// BaseURL is the NotebookLM homepage
	BaseURL = "https://notebooklm.google.com/"

	// AntiXSSIPrefix is prepended to responses by Google
	AntiXSSIPrefix = ")]}'\\n"
)
