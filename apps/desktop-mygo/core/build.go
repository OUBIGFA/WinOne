package core

// Version is the product version; UpdateRepository is the owner/name the
// update check reads. The build sets both (build/build-desktop-mygo.ps1);
// a development build has no update source.
var (
	Version          = "0.0.1"
	UpdateRepository = ""
)
