// Package notebooklmvo defines enums for NotebookLM API
package notebooklmvo

// RPCMethod represents NotebookLM RPC method IDs (reverse-engineered)
type RPCMethod string

const (
	// Notebook operations
	RPCListNotebooks  RPCMethod = "wXbhsf"
	RPCCreateNotebook RPCMethod = "CCqFvf"
	RPCGetNotebook    RPCMethod = "rLM1Ne"
	RPCRenameNotebook RPCMethod = "s0tc2d"
	RPCDeleteNotebook RPCMethod = "WWINqb"

	// Source operations
	RPCAddSource     RPCMethod = "izAoDd"
	RPCAddSourceURL  RPCMethod = "izAoDd"
	RPCAddSourceFile RPCMethod = "o4cbdc"
	RPCDeleteSource  RPCMethod = "tGMBJ"
	RPCGetSource     RPCMethod = "hizoJc"
	RPCRenameSource  RPCMethod = "BPnFVd"

	// Studio/Artifact operations
	RPCCreateAudio    RPCMethod = "AHyHrd"
	RPCCreateVideo    RPCMethod = "R7cb6c"
	RPCPollStudio     RPCMethod = "gArtLc"
	RPCCreateArtifact RPCMethod = "xpWGLf"
	RPCListArtifacts  RPCMethod = "gArtLc"
	RPCDeleteArtifact RPCMethod = "j7mI7e"
)

// StudioContentType represents artifact types
type StudioContentType int

const (
	ContentTypeAudio       StudioContentType = 1
	ContentTypeReport      StudioContentType = 2
	ContentTypeVideo       StudioContentType = 3
	ContentTypeQuiz        StudioContentType = 4
	ContentTypeMindMap     StudioContentType = 5
	ContentTypeInfographic StudioContentType = 7
	ContentTypeSlideDeck   StudioContentType = 8
	ContentTypeDataTable   StudioContentType = 9
)

// AudioFormat represents audio generation formats
type AudioFormat int

const (
	AudioFormatDeepDive AudioFormat = 1
	AudioFormatBrief    AudioFormat = 2
	AudioFormatCritique AudioFormat = 3
	AudioFormatDebate   AudioFormat = 4
)

// AudioLength represents audio length options
type AudioLength int

const (
	AudioLengthShort   AudioLength = 1
	AudioLengthDefault AudioLength = 2
	AudioLengthLong    AudioLength = 3
)

// VideoFormat represents video generation formats
type VideoFormat int

const (
	VideoFormatBriefing VideoFormat = 1
	VideoFormatTutorial VideoFormat = 2
)

// VideoStyle represents video style options
type VideoStyle int

const (
	VideoStyleClassroom   VideoStyle = 1
	VideoStyleWhiteboard  VideoStyle = 2
	VideoStyleConversaton VideoStyle = 3
)
