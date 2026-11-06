package installation

// File handles are flushed before atomic replacement. Windows does not expose
// the Unix directory-fsync operation through os.File.
func syncDir(string) error { return nil }
