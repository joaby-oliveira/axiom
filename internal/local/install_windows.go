package local

// PublishNew atomically installs a staged entry without replacing any object.
func (d AnchoredDirectory) PublishNew(staged, destination string) error {
	if err := d.StillAtPath(); err != nil {
		return err
	}
	return renameNoReplace(d.root, staged, destination)
}

// CreateOwnedDirectory creates missing private components and opens the exact
// authorized directory. Existing permissions and unknown contents are preserved.
func CreateOwnedDirectory(path string) (AnchoredDirectory, error) {
	root, identity, err := privateRootAnchored(path)
	if err != nil {
		return AnchoredDirectory{}, err
	}
	return AnchoredDirectory{root: root, anchor: identity}, nil
}
