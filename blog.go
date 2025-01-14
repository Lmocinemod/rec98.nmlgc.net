package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var blogURLPrefix = "/blog"
var blogHP = newHostedPath("blog/", blogURLPrefix+"/static/")

// BlogVideo collects static file URLs to all encodings of a video.
type BlogVideo struct {
	VP9 template.HTML // Lossless
	VP8 template.HTML // Fallback for outdated garbage

	Alt string
}

// Body generates the <source> tags and alternate text for a video.
func (b *BlogVideo) Body() (ret template.HTML) {
	ret += template.HTML(`<source src="` + b.VP9 + `" type="video/webm">`)
	ret += template.HTML(`<source src="` + b.VP8 + `" type="video/webm">`)

	if b.Alt != "" {
		ret += template.HTML(b.Alt + ". ")
	}
	ret += template.HTML(fmt.Sprintf(`<a href="%s">Download</a>`, b.VP9))
	return ret
}

// BlogEntry describes an existing blog entry, together with information about
// its associated pushes parsed from the database.
type BlogEntry struct {
	Date         string
	Pushes       []Push
	Tags         []string
	templateName string
}

// Blog bundles all blog entries, sorted from newest to oldest.
type Blog []BlogEntry

// NewBlog parses all HTML files in the blog path into t, and returns a new
// sorted Blog. funcs can be used to add any template functions that rely on
// a Blog instance.
func NewBlog(t *template.Template, pushes tPushes, tags tBlogTags, funcs func(b Blog) map[string]interface{}) (ret Blog) {
	// Unlike Go's own template.ParseGlob, we want to prefix template names
	// with their local path.
	templates, err := filepath.Glob(filepath.Join(blogHP.LocalPath, "*.html"))
	FatalIf(err)
	sort.Slice(templates, func(i, j int) bool { return templates[i] > templates[j] })
	for _, tmpl := range templates {
		basename := filepath.Base(tmpl)
		date := strings.TrimSuffix(basename, path.Ext(basename))
		ret = append(ret, BlogEntry{
			Date:         date,
			Pushes:       pushes.DeliveredAt(date),
			Tags:         tags[date],
			templateName: tmpl,
		})
	}
	t.Funcs(funcs(ret))
	for _, tmpl := range templates {
		buf, err := ioutil.ReadFile(tmpl)
		FatalIf(err)
		template.Must(t.New(tmpl).Parse(string(buf)))
	}
	return
}

// FindEntryByString looks for and returns a potential blog entry posted
// during the given ISO 8601-formatted date, or nil if there is none.
func (b Blog) FindEntryByString(date string) (*BlogEntry, error) {
	// Note that we don't use sort.SearchStrings() here, since we're sorted
	// in descending order!
	i := sort.Search(len(b), func(i int) bool { return b[i].Date <= date })
	if i >= len(b) || b[i].Date != date {
		return nil, eNoPost{date}
	}
	return &b[i], nil
}

// FindEntryByTime looks for and returns a potential blog entry posted during
// the date of the given Time instance, or nil if there is none.
func (b Blog) FindEntryByTime(date time.Time) *BlogEntry {
	entry, _ := b.FindEntryByString(date.Format("2006-01-02"))
	return entry
}

// FindEntryForPush looks for and returns a potential blog entry which
// summarizes the given Push.
func (b Blog) FindEntryForPush(p Push) *BlogEntry {
	return b.FindEntryByTime(p.Delivered)
}

// PostDot contains everything handed to a blog template as the value of dot.
type PostDot struct {
	Date       string      // ISO 8601-formatted date
	HostedPath *hostedPath // Value of [blogHP]
	DatePrefix string      // Date prefix for potential post-specific files
	// Generates [HostedPath.URLPrefix] + [DatePrefix]
	PostFileURL func(fn string) template.HTML
	Video       func(fn string, alt string) *BlogVideo
}

// Post bundles the rendered HTML body of a post with all necessary header
// data.
type Post struct {
	Date     string
	Time     time.Time // Full post time
	PushIDs  []PushID
	FundedBy []CustomerID
	Diffs    []DiffInfo
	Tags     []string
	Filters  []string
	Body     template.HTML
}

type eNoPost struct {
	date string
}

func (e eNoPost) Error() string {
	return fmt.Sprintf("no blog entry posted on %s", e.date)
}

// Render builds a new Post instance from e.
func (e BlogEntry) Render(filters []string) Post {
	var b strings.Builder
	datePrefix := e.Date + "-"
	postFileURL := func(fn string) template.HTML {
		return template.HTML(blogHP.VersionURLFor(datePrefix + fn))
	}
	ctx := PostDot{
		Date:        e.Date,
		HostedPath:  blogHP,
		DatePrefix:  datePrefix,
		PostFileURL: postFileURL,
		Video: func(fn string, alt string) *BlogVideo {
			return &BlogVideo{
				VP9: postFileURL(fn + ".webm"),
				VP8: postFileURL(fn + "-vp8.webm"),
				Alt: alt,
			}
		},
	}
	pagesExecute(&b, e.templateName, &ctx)

	post := Post{
		Date:    e.Date,
		Tags:    e.Tags,
		Filters: filters,
		Body:    template.HTML(b.String()),
	}
	if e.Pushes != nil {
		post.Time = e.Pushes[0].Delivered
	} else {
		post.Time = DateInDevLocation(e.Date).Time
	}

	for i := len(e.Pushes) - 1; i >= 0; i-- {
		push := &e.Pushes[i]
		post.PushIDs = append(post.PushIDs, push.ID)
		post.Diffs = append(post.Diffs, push.Diff)
		post.FundedBy = append(post.FundedBy, push.FundedBy()...)
	}
	RemoveDuplicates(&post.Diffs)
	RemoveDuplicates(&post.FundedBy)
	return post
}

// GetPost returns the post that was originally posted on the given date.
func (b Blog) GetPost(date string) (*Post, error) {
	entry, err := b.FindEntryByString(date)
	if err != nil {
		return nil, err
	}
	post := entry.Render([]string{})
	return &post, nil
}

// Posts renders all blog posts that match the given slice of filters. Pass an
// empty slice to get all posts.
func (b Blog) Posts(filters []string) chan Post {
	ret := make(chan Post)
	go func() {
		for _, entry := range b {
			filtersSeen := 0
			for _, tag := range entry.Tags {
				for _, filter := range filters {
					if filter == tag {
						filtersSeen++
					}
				}
			}
			if filtersSeen == len(filters) {
				ret <- entry.Render(filters)
			}
		}
		close(ret)
	}()
	return ret
}

// PostLink returns a nicely formatted link to the given blog post.
func (b Blog) PostLink(date string, text string) template.HTML {
	_, err := b.FindEntryByString(date)
	FatalIf(err)
	return template.HTML(fmt.Sprintf(
		`<a href="%s/%s">📝 %s</a>`, blogURLPrefix, date, text,
	))
}
