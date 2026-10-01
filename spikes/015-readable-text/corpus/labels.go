package corpus

// Labels were written by hand after reading the saved fixtures (2026-09-28).
// Must = phrases from the main text (start, middle and end where possible).
// MustNot = boilerplate: menus, footers, related-article boxes, cookie text.
// Phrases avoid curly quotes; matching is case-insensitive with whitespace
// collapsed and Arabic yeh/kaf mapped to the Persian letters. Derived
// (legacy-encoding) fixtures reuse the labels of their source.
type label struct {
	Title   string
	Must    []string
	MustNot []string
	Row     [][]string
}

var labels = map[string]label{
	"news-bbc-en": {Title: "RAF Fairford",
		Must: []string{
			"raf fairford is located in the cotswolds",
			"the bbc understands the operation was not intelligence led",
			"i believe that my 999 call foiled whatever plans they had",
		},
		MustNot: []string{
			"thousands of illegal ghost plates on cars revealed by new cameras",
			"the bbc is not responsible for the content of external sites",
			"anne hathaway and jessica chastain star in a psychological thriller",
			"parental guidance",
		}},
	"news-guardian-en": {Title: "cape town divided over plan to remove its monkeys",
		Must: []string{
			"a 2025 census found that the population had grown by a third since 2000",
			"residents can no longer barbecue or host garden parties",
			"a baboon was shot straight through the chest",
		},
		MustNot: []string{
			"original reporting and incisive analysis, direct from the guardian every morning",
			"tax strategy",
			"wordiply",
			"most viewed",
		}},
	"news-tagesschau-de": {Title: "Ausschluss arabischer Parteien",
		Must: []string{
			"die beiden wichtigsten arabisch geführten wahllisten sollen bei der knesset-wahl am 27. oktober nicht antreten dürfen",
			"er ist lehrer aus abu gosh",
			"ich glaube, dass das gericht die entscheidung des wahlausschusses aufheben wird",
		},
		MustNot: []string{
			"jüdischer politiker wechselt zu arabischer partei",
			"tagesschau-kanal bei whatsapp",
			"rundfunk berlin-brandenburg",
			"untermenü inland einblenden",
		}},
	"news-spiegel-de": {Title: "ungewöhnlich warmes Herbstwetter",
		Must: []string{
			"der deutsche wetterdienst (dwd) erwartet",
			"vom niederrhein bis nach schleswig-holstein komme es zeitweise zu regenfällen",
			"die höchsttemperaturen lägen dann nur noch zwischen 18 und 23 grad",
		},
		MustNot: []string{
			"brutto-netto-rechner",
			"vertrag kündigen",
			"cookies & tracking",
			"zur merkliste hinzufügen",
		}},
	"news-heise-de": {Title: "Solarstromproduktion übertrifft",
		Must: []string{
			"bis zum 27. september seien 90,2 terawattstunden solarstrom produziert worden",
			"zugleich gewännen batteriespeicher an bedeutung",
			"kleine, meist privat installierte solaranlagen mit unter 25 kilowatt installierter leistung keine dauerhafte förderung mehr bekommen",
		},
		MustNot: []string{
			"jeden morgen der frische nachrichtenüberblick von heise online",
			"fünf vorteile von enterprise browsern",
			"das beste ferngesteuerte boot im test",
			"heise regioconcept",
		}},
	"news-bbc-fa": {Title: "پنج دهه",
		Must: []string{
			"حضور مقام‌های ایرانی در نیویورک",
			"بحران گروگان‌گیری ۴۴۴ روز طول کشید",
			"امانپور نپذیرفت و مصاحبه لغو شد",
			"تنها نماینده آمریکا که در جایگاه هیئت این کشور نشسته بود نیز سالن را ترک کرد",
		},
		MustNot: []string{
			"مطالب پرخواننده",
			"بی بی سی مسئول محتوای سایت های دیگر نیست",
			"رها اخوان، اولین زن ایرانی که کانال مانش را شنا کرد",
			"چرا فلوریدا بیشترین اعدام‌ها را در آمریکا انجام می‌دهد",
		}},
	"news-dw-fa": {Title: "جوایز امی",
		Must: []string{
			"رکورد بیشترین تعداد امی برای یک سریال کمدی",
			"استیون روت با این پیروزی در رقابت با هریسون فورد",
			"فاکس این جایزه را به‌دلیل فعالیت‌هایش در حمایت از پژوهش و درمان پارکینسون",
		},
		MustNot: []string{
			"آموزش زبان آلمانی",
			"dw را به عنوان منبع ترجیحی در google اضافه کنید",
			"اینترنت بدون سانسور با سایفون",
			"دویچه وله فارسی را در اینستاگرام دنبال کنید",
		}},

	"wiki-en": {Title: "Bitcoin",
		Must: []string{
			"is the first decentralized cryptocurrency",
			"the domain name bitcoin.org was registered on 18 august 2008",
			"all transactions are public on the blockchain",
			"regulated bitcoin funds also allow exposure to the asset",
		},
		MustNot: []string{
			"create account log in",
			"text is available under the creative commons attribution-sharealike 4.0 license",
			"hidden categories",
			"switch to legacy parser",
		}},
	"wiki-de": {Title: "Bitcoin",
		Must: []string{
			"bitcoin ist die erste und am weltmarkt stärkste kryptowährung",
			"digitales geld ohne zentrale kontrollinstanz zu etablieren",
			"ein mining-pool ist die zusammenlegung der rechenleistung von mining-hardware",
		},
		MustNot: []string{
			"quelltext bearbeiten",
			"benutzerkonto erstellen",
			"versteckte kategorien",
			"stellungnahme zu cookies",
		}},
	"wiki-fa": {Title: "بیت‌کوین",
		Must: []string{
			"هیچ نهادی بیت‌کوین را کنترل نمی‌کند",
			"شبکهٔ بیت‌کوین همتا به همتا است",
			"مصرف بالای انرژی توسط شبکهٔ بیت‌کوین همواره یکی از موارد انتقاد به بیت‌کوین بوده است",
		},
		MustNot: []string{
			"ساخت حساب ورود",
			"رده‌های پنهان",
			"بیانیهٔ کوکی",
			"ویکی‌پدیا® علامتی تجاری",
		}},

	"docs-go-effective": {Title: "Effective Go",
		Must: []string{
			"go is an open-source programming language that focuses on simplicity, reliability, and efficiency",
			"reads a go program and emits the source in a standard style of indentation and vertical alignment",
			"do not communicate by sharing memory; instead, share memory by communicating",
			"a useful web server in a few lines of code plus some data-driven html text",
		},
		MustNot: []string{
			"go.dev uses cookies from google to deliver and enhance the quality of its services",
			"common problems companies solve with go",
			"brand guidelines",
			"meet other local go developers",
		}},
	"docs-mdn-en": {Title: "table",
		Must: []string{
			"represents tabular data",
			"whose value clearly and concisely describes the table's purpose",
			"specifies the horizontal alignment of the table within its parent element",
		},
		MustNot: []string{
			"your blueprint for a better internet",
			"check out the video course from scrimba, our partner",
			"border-radius generator",
			"community participation guidelines",
		}},
	"docs-mdn-de": {Title: "HTML-Tabellenelement",
		Must: []string{
			"tabellarische daten",
			"dessen wert den zweck der tabelle klar und prägnant beschreibt",
			"gibt die horizontale ausrichtung der",
		},
		MustNot: []string{
			"der bauplan für ein besseres internet",
			"check out the video course from scrimba, our partner",
			"telemetrie-einstellungen",
			"border-radius generator",
		}},

	"blog-go-slices": {Title: "Go Slices: usage and internals",
		Must: []string{
			"slices are analogous to arrays in other languages, but have some unusual properties",
			"an array type definition specifies a length and an element type",
			"this is left as an exercise for the reader",
		},
		MustNot: []string{
			"go.dev uses cookies from google to deliver and enhance the quality of its services",
			"common problems companies solve with go",
			"brand guidelines",
		}},
	"blog-rsc-vgo": {Title: "Semantic Import Versioning",
		Must: []string{
			"how do you deploy an incompatible change to an existing package?",
			"i call this convention semantic import versioning",
			"sam boyer talked at gophercon 2017 about how package managers moderate our social interactions",
		},
		MustNot: []string{
			"thoughts and links about programming, by russ cox",
		}},
	"blog-cloudflare": {Title: "Turnstile Spin",
		Must: []string{
			"cloudflare declared itself free from captchas with the launch of turnstile",
			"turnstile now processes about three billion verifications on a typical weekday",
			"spin does not send your application code to cloudflare",
			"the dashboard has recorded more than 65,000 successful spin widget creations",
		},
		MustNot: []string{
			"subscribe to receive notifications of new posts",
			"argo smart routing",
			"report security issues",
			"project fairshot",
		}},
	"article-verbraucherzentrale-de": {Title: "Cookies kontrollieren und verwalten",
		Must: []string{
			"bei fast jeder website, die sie besuchen, werden kleine datensätze in ihrem browser auf ihrem gerät gespeichert",
			"nach einer entscheidung des europäischen gerichtshofs (az. c-673/17)",
			"mit hilfe von cookies können unternehmen ihr surfverhalten theoretisch über jahre verfolgen",
		},
		MustNot: []string{
			"santander bank: drängt sie kreditkund:innen in eine versicherung?",
			"wählen sie ihr bundesland, um regionale angebote und services zu sehen",
			"renditerechner: investieren sie ihr geld clever",
			"umfrage: ärger mit lemonswan?",
		}},

	"forum-hn": {Title: "OpenAI's board has fired Sam Altman",
		Must: []string{
			"our poor single-core server process has smoke coming out its ears",
			"the board struggle reflected a cultural clash at the organization",
			"sam strikes me as the type of founder who would never sell out",
			"microsoft's own employees use macs during demos",
		},
		MustNot: []string{
			"apply to yc",
			"submit login",
		}},
	"forum-golangbridge": {Title: "list2regexp",
		Must: []string{
			"given the following list of strings",
			"list2regexp will return the following pattern",
			"just wondering how you intend for people to use it",
			"this project grew out of sriracha, where it is used to match large ip lists",
		},
		MustNot: []string{
			"best viewed with javascript enabled",
		}},
	"github-readme": {Title: "goquery",
		Must: []string{
			"goquery brings a syntax and a set of features similar to jquery to the go language",
			"it is the caller's responsibility to ensure that the source document provides utf-8 encoded html",
			"please search existing issues before opening a new one",
		},
		MustNot: []string{
			"you signed in with another tab or window",
			"github copilot",
			"do not share my personal information",
			"enterprise-grade 24/7 support",
		}},

	"table-wiki-population": {Title: "List of countries and dependencies by population",
		Must: []string{
			"this article reflects continually changing information from hundreds of sources",
			"population distribution by country in june-july 2025",
		},
		MustNot: []string{
			"create account log in",
			"hidden categories",
			"text is available under the creative commons attribution-sharealike 4.0 license",
		},
		Row: [][]string{{"germany", "83,298,787"}, {"japan", "122,650,000"}, {"iceland", "396,500"}}},
	"table-wiki-fa-population": {Title: "فهرست کشورها",
		Must: []string{
			"این نوشتار فهرستی از کشورها و قلمروهای وابسته بر پایهٔ جمعیت است",
			"بریتانیا و قلمروهای وابسته به آن به‌صورت یک نهاد واحد",
		},
		MustNot: []string{
			"ساخت حساب ورود",
			"رده‌های پنهان",
			"بیانیهٔ کوکی",
		},
		Row: [][]string{{"آلمان", "۸۴٬۶۰۷٬۰۱۶"}, {"ایران", "۹۲٬۸۲۴٬۰۴۰"}, {"ژاپن", "۱۲۳٬۹۹۰٬۰۰۰"}}},
	"table-ecb-rates": {Title: "Euro foreign exchange reference rates",
		Must: []string{
			"the reference rates are usually updated at around 16:00 cet every working day",
			"using the rates for transaction purposes is strongly discouraged",
			"the ecb last published a eur/rub reference rate on 1 march 2022",
		},
		MustNot: []string{
			"i understand and i accept the use of cookies",
			"we use the anonymous data provided by cookies",
			"ecb scholarship for ukrainian graduates",
			"girls' it bootcamp",
		},
		Row: [][]string{{"usd", "us dollar", "1.1403"}, {"jpy", "japanese yen", "179.70"}, {"gbp", "pound sterling", "0.86045"}}},
	"table-govuk-holidays": {Title: "UK bank holidays",
		Must: []string{
			"the next bank holiday in england and wales is",
			"your employer does not have to give you paid leave on bank or public holidays",
			"if a bank holiday is on a weekend",
		},
		MustNot: []string{
			"we use some essential cookies to make this website work",
			"accept additional cookies",
			"is this page useful?",
			"births, death, marriages and care",
		},
		Row: [][]string{{"30 august", "monday", "summer bank holiday"}, {"26 march", "friday", "good friday"}, {"27 december", "monday", "christmas day (substitute day)"}}},

	"banner-govuk": {Title: "Apply online for a UK passport",
		Must: []string{
			"use this service to apply for, renew, replace or update your passport and pay for it online",
			"do not book travel until you have a valid passport",
			"you can pick up a paper passport application form from your local post office",
		},
		MustNot: []string{
			"we use some essential cookies to make this website work",
			"accept additional cookies",
			"is this page useful?",
			"births, death, marriages and care",
		}},
	"gov-destatis-de": {Title: "Verbraucherpreisindex",
		Must: []string{
			"der verbraucherpreisindex misst monatlich die durchschnittliche preisentwicklung aller waren und dienstleistungen",
			"der rund 700 güterarten umfasst",
			"der verbraucherpreisindex dient insbesondere zur messung der geldwertstabilität",
		},
		MustNot: []string{
			"der durchschnittsmensch in deutschland",
			"gustav-stresemann-ring 11",
			"bürokratiekosten justiz und rechtspflege",
			"erklärung zur barrierefreiheit",
		}},
	"gov-bundesbank-de": {Title: "Wechselkurse",
		Must: []string{
			"der themenbereich beinhaltet informationen zu devisenkursen und effektiven wechselkursen",
			"zeitreihe mit den euro-referenzkursen der europäischen zentralbank zum us-dollar",
			"ein zentrales suchfeld ermöglicht eine schnelle und direkte suche nach zeitreihen",
		},
		MustNot: []string{
			"die bundesbank unterstützt den wandel hin zu einer kohlenstoffarmen wirtschaft",
			"suchen sie im aktuell gültigen verzeichnis der bankleitzahlen",
			"ihre anrufe nehmen wir montags bis freitags von 8 uhr bis 16 uhr gern entgegen",
			"informationsfreiheitsgesetz",
		}},

	"js-coinmarketcap": {Title: "Bitcoin price today",
		Must: []string{
			"bitcoin is a decentralized cryptocurrency originally described in a 2008 whitepaper",
			"laszlo hanyecz traded 10,000 bitcoins for two pizzas",
			"the live bitcoin price today is $83,205.70 usd",
		},
		MustNot: []string{
			"fear and greed index",
			"cmc agent hub",
			"cookie preferences cookie policy",
		}},
	// The HTML is an empty app shell: no main text to find.
	"js-excalidraw": {Title: "Excalidraw"},

	"large-go-spec": {Title: "The Go Programming Language Specification",
		Must: []string{
			"this is the reference manual for the go programming language",
			"a type determines a set of values together with operations and methods specific to those values",
			"a compiler may make it illegal to declare a variable inside a function body if the variable is never used",
		},
		MustNot: []string{
			"go.dev uses cookies from google to deliver and enhance the quality of its services",
			"common problems companies solve with go",
			"brand guidelines",
		}},
	"large-whatwg-html": {Title: "HTML Standard",
		Must: []string{
			"this specification is limited to providing a semantic-level markup language",
			"implementations must act as if they used the following state machine to tokenize html",
			"this registration is for community review and will be submitted to the iesg",
			"thanks to the wicg for incubating the sanitizer api",
		},
		MustNot: []string{
			"chat on matrix",
			"snapshot as of this commit",
		}},

	"legacy-aozora-sjis": {Title: "こころ",
		Must: []string{
			"その時私はまだ若々しい書生であった",
			"電報には母が病気だからと断ってあったけれども友達はそれを信じなかった",
			"私は妻には何にも知らせたくないのです",
		},
		MustNot: []string{
			"入力、校正、制作にあたったのは、ボランティアの皆さんです",
		}},
}

func init() {
	for i := range Pages {
		p := &Pages[i]
		src := p.ID
		if p.From != "" {
			src = p.From
		}
		if l, ok := labels[src]; ok {
			p.Title, p.Must, p.MustNot, p.Row = l.Title, l.Must, l.MustNot, l.Row
		}
	}
}
