package common

// SnapshotRecorder captures a file before a built-in write/edit mutation.
// Record reports whether it added the turn's first snapshot for the path;
// Discard removes that snapshot when the attempted mutation fails.
type SnapshotRecorder interface {
	Record(path string) bool
	Discard(path string)
}
