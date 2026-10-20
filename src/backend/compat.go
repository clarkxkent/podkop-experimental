package podkop

// compatItem is a thing the backend does only because the shell backend of podkop 0.7,
// or the page of podkop 0.7, is or may be there.
type compatItem struct {
	// Name is the mark of the places that do it: a comment "compat: <name>".
	Name string
	// Why says whom it serves.
	Why string
	// Until says when it can go.
	Until string
}

// compat is the list of what is kept for podkop 0.7: one place to look when it can go.
// Each item is marked where it is done; a test holds the marks and the list to each other
// (compat_test.go).
var compat = []compatItem{
	{
		Name:  "clean-shutdown",
		Why:   "podkop.settings.shutdown_correctly is set back to 1 when dnsmasq is given back: the shell backend sets dnsmasq up at its start only after a clean stop",
		Until: "the shell backend cannot be installed in place of this one",
	},
	{
		Name:  "rule-set-files",
		Why:   "the rule-set files are removed at the stop: the shell backend takes a file it finds for one of its own",
		Until: "the shell backend cannot be installed in place of this one",
	},
	{
		Name:  "dnsmasq-backup-keys",
		Why:   "the podkop_* keys the shell backend left on dnsmasq mark a dnsmasq it took, which the backend gives back",
		Until: "no router has a dnsmasq taken by the shell backend",
	},
	{
		Name:  "output-mark",
		Why:   "mangle_output matches the mark 0x00200000, which nothing sets: the table is the shell backend's rule by rule, and its nft text is compared with it",
		Until: "the table no longer has to equal the shell backend's",
	},
	{
		Name:  "page-answers",
		Why:   "the commands print the JSON and the text of the shell backend: the page of podkop 0.7 reads them",
		Until: "the page asks a typed API",
	},
	{
		Name:  "page-features",
		Why:   "get_sing_box_features names what the page of podkop 0.7.24 asks for before it shows an option",
		Until: "the page asks a typed API",
	},
}
