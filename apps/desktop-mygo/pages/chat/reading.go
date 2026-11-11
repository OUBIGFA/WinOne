package chat

// Spacing of the transcript. The look of a reply's text itself is the kit's
// ReadingTheme, shared with the side panel's Markdown preview.
const (
	// readingSize is a user message's text, the size of the replies.
	readingSize = 15.0 / 14
	// turnGap separates one round from the next; stepGap, the parts of a
	// round: the message, its work process, its reply.
	turnGap = 32
	stepGap = 10
)
