package main

import (
	"html/template"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

/// HTML templates
/// --------------

var pages = template.Must(template.ParseGlob("*.html"))

// executeTemplate wraps template execution on [pages], logging any errors
// using the facilities from package log.
func executeTemplate(wr io.Writer, name string, data interface{}) {
	if err := pages.ExecuteTemplate(wr, name, data); err != nil {
		log.Println(wr, err)
	}
}

func htmlWrap(handler func(wr http.ResponseWriter, req *http.Request)) http.Handler {
	return http.HandlerFunc(func(wr http.ResponseWriter, req *http.Request) {
		executeTemplate(wr, "header.html", nil)
		handler(wr, req)
		executeTemplate(wr, "footer.html", nil)
	})
}

/// --------------

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <ReC98 repository path/URL>\n", os.Args[0])
	}

	err := optimalClone(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Done.")

	master, err = repo.ResolveRevision("master")
	if err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()

	staticSrv := http.FileServer(http.Dir("static/"))

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", staticSrv))
	r.Handle("/favicon.ico", staticSrv)
	r.Handle("/", htmlWrap(indexHandler))
	log.Fatal(http.ListenAndServe(":8098", r))
}
