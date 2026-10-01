package corpus

// Pages is the fixture set. Phrases were written after reading the saved
// fixtures; a refetch gets newer page versions, so phrases may need updating.
var Pages = []Page{
	// ---- News ----
	{ID: "news-bbc-en", Group: "news", Lang: "en",
		URL: "https://www.bbc.co.uk/news/articles/c85ydnwqpzyzo"},
	{ID: "news-guardian-en", Group: "news", Lang: "en",
		URL: "https://www.theguardian.com/environment/2026/sep/27/baboons-cape-town-divided-over-plan-to-remove-its-monkeys-aoe"},
	{ID: "news-tagesschau-de", Group: "news", Lang: "de",
		URL: "https://www.tagesschau.de/ausland/asien/israel-wahlen-parteien-arabisch-100.html"},
	{ID: "news-spiegel-de", Group: "news", Lang: "de",
		URL: "https://www.spiegel.de/panorama/wetter-in-deutschland-ungewoehnlich-warmes-herbstwetter-mit-bis-zu-31-grad-a-8a275881-6b48-460c-a9e1-f20595228f78"},
	{ID: "news-heise-de", Group: "news", Lang: "de",
		URL: "https://www.heise.de/news/Solarstromproduktion-uebertrifft-bereits-jetzt-Vorjahreswert-11467510.html"},
	{ID: "news-bbc-fa", Group: "news", Lang: "fa",
		URL: "https://www.bbc.com/persian/articles/cmrl6644xz14o"},
	{ID: "news-dw-fa", Group: "news", Lang: "fa",
		URL: "https://www.dw.com/fa-ir/%D8%A7%D8%B9%D9%84%D8%A7%D9%85-%D8%AC%D9%88%D8%A7%DB%8C%D8%B2-%D8%A7%D9%85%DB%8C-%DB%B2%DB%B0%DB%B2%DB%B6%D8%9B-%D8%B4%D8%A8%DB%8C-%D8%A8%D8%B1%D8%A7%DB%8C-%D8%B1%DA%A9%D9%88%D8%B1%D8%AF%D9%87%D8%A7-%D9%88-%D8%A8%D8%A7%D8%B2%DA%AF%D8%B4%D8%AA%E2%80%8C%D9%87%D8%A7/a-79272634"},

	// ---- Wikipedia ----
	{ID: "wiki-en", Group: "wiki", Lang: "en", URL: "https://en.wikipedia.org/wiki/Bitcoin"},
	{ID: "wiki-de", Group: "wiki", Lang: "de", URL: "https://de.wikipedia.org/wiki/Bitcoin"},
	{ID: "wiki-fa", Group: "wiki", Lang: "fa",
		URL: "https://fa.wikipedia.org/wiki/%D8%A8%DB%8C%D8%AA%E2%80%8C%DA%A9%D9%88%DB%8C%D9%86"},

	// ---- Documentation ----
	{ID: "docs-go-effective", Group: "docs", Lang: "en", URL: "https://go.dev/doc/effective_go"},
	{ID: "docs-mdn-en", Group: "docs", Lang: "en",
		URL: "https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/table"},
	{ID: "docs-mdn-de", Group: "docs", Lang: "de",
		URL: "https://developer.mozilla.org/de/docs/Web/HTML/Reference/Elements/table"},

	// ---- Blogs ----
	{ID: "blog-go-slices", Group: "blog", Lang: "en", URL: "https://go.dev/blog/slices-intro"},
	{ID: "blog-rsc-vgo", Group: "blog", Lang: "en", URL: "https://research.swtch.com/vgo-import"},
	{ID: "blog-cloudflare", Group: "blog", Lang: "en", URL: "https://blog.cloudflare.com/turnstile-spin/"},

	// ---- Forums and code hosting ----
	{ID: "forum-hn", Group: "forum", Lang: "en", URL: "https://news.ycombinator.com/item?id=38309611"},
	{ID: "forum-golangbridge", Group: "forum", Lang: "en",
		URL: "https://forum.golangbridge.org/t/announcing-list2regexp-generate-regular-expressions-to-match-lists-of-strings/42579"},
	{ID: "github-readme", Group: "forum", Lang: "en", URL: "https://github.com/PuerkitoBio/goquery"},

	// ---- Tables ----
	{ID: "table-wiki-population", Group: "table", Lang: "en",
		URL: "https://en.wikipedia.org/wiki/List_of_countries_and_dependencies_by_population"},
	{ID: "table-wiki-fa-population", Group: "table", Lang: "fa",
		URL: "https://fa.wikipedia.org/wiki/%D9%81%D9%87%D8%B1%D8%B3%D8%AA_%DA%A9%D8%B4%D9%88%D8%B1%D9%87%D8%A7_%D8%A8%D8%B1_%D9%BE%D8%A7%DB%8C%D9%87_%D8%AC%D9%85%D8%B9%DB%8C%D8%AA"},
	{ID: "table-ecb-rates", Group: "table", Lang: "en",
		URL: "https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/html/index.en.html"},
	{ID: "table-govuk-holidays", Group: "table", Lang: "en", URL: "https://www.gov.uk/bank-holidays"},

	// ---- Cookie banners in the HTML (gov.uk, ECB) and German government pages ----
	{ID: "banner-govuk", Group: "banner", Lang: "en", URL: "https://www.gov.uk/apply-renew-passport"},
	{ID: "gov-destatis-de", Group: "gov", Lang: "de",
		URL: "https://www.destatis.de/DE/Themen/Wirtschaft/Preise/Verbraucherpreisindex/_inhalt.html"},
	{ID: "gov-bundesbank-de", Group: "gov", Lang: "de",
		URL: "https://www.bundesbank.de/de/statistiken/wechselkurse"},
	{ID: "article-verbraucherzentrale-de", Group: "blog", Lang: "de",
		URL: "https://www.verbraucherzentrale.de/wissen/digitale-welt/datenschutz/cookies-kontrollieren-und-verwalten-11996"},

	// ---- JavaScript-heavy ----
	{ID: "js-coinmarketcap", Group: "js", Lang: "en", URL: "https://coinmarketcap.com/currencies/bitcoin/"},
	{ID: "js-excalidraw", Group: "js", Lang: "en", URL: "https://excalidraw.com/"},

	// ---- Large ----
	{ID: "large-go-spec", Group: "large", Lang: "en", URL: "https://go.dev/ref/spec"},
	{ID: "large-whatwg-html", Group: "large", Lang: "en", URL: "https://html.spec.whatwg.org/"},

	// ---- Legacy encodings ----
	{ID: "legacy-aozora-sjis", Group: "legacy", Lang: "ja",
		URL: "https://www.aozora.gr.jp/cards/000148/files/773_14560.html"},
	{ID: "legacy-bbc-fa-1256-meta", Group: "legacy", Lang: "fa",
		From: "news-bbc-fa", Encoding: "windows-1256", Meta: true},
	{ID: "legacy-bbc-fa-1256-header", Group: "legacy", Lang: "fa",
		From: "news-bbc-fa", Encoding: "windows-1256", Meta: false},
	{ID: "legacy-tagesschau-1252-meta", Group: "legacy", Lang: "de",
		From: "news-tagesschau-de", Encoding: "windows-1252", Meta: true},
}

func ByID(id string) *Page {
	for i := range Pages {
		if Pages[i].ID == id {
			return &Pages[i]
		}
	}
	return nil
}
