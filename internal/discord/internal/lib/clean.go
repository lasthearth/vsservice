package lib

import "regexp"

var (
	markdownHeaderPattern = regexp.MustCompile(`^#{1,6}\s+`)
	codeBlockPattern      = regexp.MustCompile("```[\\s\\S]*?```")
	inlineCodePattern     = regexp.MustCompile("`[^`]*`")
	spoilerPattern        = regexp.MustCompile("\\|\\|[\\s\\S]*?\\|\\|")
	strikethroughPattern  = regexp.MustCompile("~~(.*?)~~")
	boldPattern           = regexp.MustCompile("\\*{1,3}(.*?)\\*{1,3}")
	italicPattern         = regexp.MustCompile("_{1,3}(.*?)_{1,3}")
	linkPattern           = regexp.MustCompile("\\[([^\\]]*)\\]\\([^)]*\\)")
	autoLinkPattern       = regexp.MustCompile("<(https?://[^>]+)>")
	blockquotePattern     = regexp.MustCompile("^\\s*>{1,3}\\s?")
	escapedPattern        = regexp.MustCompile("\\\\([*_~`|\\[\\]()()<>#+\\-=.!{}])")
	blankLinesPattern     = regexp.MustCompile("\\n{3,}")
	htmlTagPattern        = regexp.MustCompile(`<[^>]*>`)
	whitespacePattern     = regexp.MustCompile(`\s+`)
)

// CleanContent removes Discord Markdown markup from a message.
func CleanContent(content string) string {
	content = markdownHeaderPattern.ReplaceAllString(content, "")
	content = codeBlockPattern.ReplaceAllString(content, "")
	content = inlineCodePattern.ReplaceAllString(content, "")
	content = spoilerPattern.ReplaceAllString(content, "")
	content = strikethroughPattern.ReplaceAllString(content, "$1")
	content = boldPattern.ReplaceAllString(content, "$1")
	content = italicPattern.ReplaceAllString(content, "$1")
	content = linkPattern.ReplaceAllString(content, "$1")
	content = autoLinkPattern.ReplaceAllString(content, "$1")
	content = blockquotePattern.ReplaceAllString(content, "")
	content = escapedPattern.ReplaceAllString(content, "$1")
	content = blankLinesPattern.ReplaceAllString(content, "\n\n")
	return content
}

// StripHTML removes HTML tags and collapses whitespace.
func StripHTML(html string) string {
	if html == "" {
		return ""
	}

	plain := htmlTagPattern.ReplaceAllString(html, " ")
	plain = whitespacePattern.ReplaceAllString(plain, " ")
	return plain
}

// Truncate cuts a string to maxLength with an ellipsis.
func Truncate(value string, maxLength int) string {
	if value == "" || len(value) <= maxLength {
		return value
	}

	return value[:maxLength-3] + "..."
}
