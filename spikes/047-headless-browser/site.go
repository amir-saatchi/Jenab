package main

// The spike's own pages and its canaries. The site is served on loopback
// under a test name that only the proxy knows (site.spike047.test), which
// is a private-network exception; the canaries listen on loopback and the
// LAN address, which are not. Every hit on a canary is a request that got
// past the proxy.

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
)

const siteName = "site.spike047.test"

// Hit is one request a canary saw.
type Hit struct {
	At       time.Time
	Listener string
	Remote   string
	What     string
}

type Canaries struct {
	LAN                  string // this PC's LAN address
	Loop, LanTCP         int    // ports: 127.0.0.1 and [::1] share Loop
	STUN, STUNLoop, TURN int

	mu   sync.Mutex
	hits []Hit
}

func lanAddr() string {
	ifs, _ := net.Interfaces()
	var best string
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, _ := i.Addrs()
		for _, a := range as {
			p, err := netip.ParsePrefix(a.String())
			if err != nil || !p.Addr().Is4() || !p.Addr().IsPrivate() {
				continue
			}
			if strings.HasPrefix(p.Addr().String(), "192.168.") {
				return p.Addr().String()
			}
			if best == "" {
				best = p.Addr().String()
			}
		}
	}
	return best
}

func (c *Canaries) add(listener, remote, what string) {
	c.mu.Lock()
	c.hits = append(c.hits, Hit{time.Now(), listener, remote, what})
	c.mu.Unlock()
}

// Take returns the hits so far and clears them.
func (c *Canaries) Take() []Hit {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := c.hits
	c.hits = nil
	return h
}

func (c *Canaries) Start() error {
	c.LAN = lanAddr()
	handler := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			what := r.Method + " " + r.URL.Path
			if r.Header.Get("Upgrade") != "" {
				what += " (upgrade " + r.Header.Get("Upgrade") + ")"
			}
			c.add(name, r.RemoteAddr, what)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			if strings.Contains(r.URL.Path, "-es") {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: canary\n\n")
				return
			}
			fmt.Fprint(w, "<!doctype html><title>canary</title><p>CANARY PAGE: this request got past the proxy.</p>")
		})
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	c.Loop = ln.Addr().(*net.TCPAddr).Port
	go http.Serve(ln, handler("127.0.0.1"))
	if ln6, err := net.Listen("tcp", fmt.Sprintf("[::1]:%d", c.Loop)); err == nil {
		go http.Serve(ln6, handler("[::1]"))
	}
	if c.LAN != "" {
		ll, err := net.Listen("tcp", c.LAN+":0")
		if err != nil {
			return err
		}
		c.LanTCP = ll.Addr().(*net.TCPAddr).Port
		go http.Serve(ll, handler("LAN"))
		// TURN over TCP: any connection counts
		tl, err := net.Listen("tcp", c.LAN+":0")
		if err == nil {
			c.TURN = tl.Addr().(*net.TCPAddr).Port
			go func() {
				for {
					conn, err := tl.Accept()
					if err != nil {
						return
					}
					c.add("LAN TURN/tcp", conn.RemoteAddr().String(), "connection")
					conn.Close()
				}
			}()
		}
		if u, err := net.ListenPacket("udp", c.LAN+":0"); err == nil {
			c.STUN = u.LocalAddr().(*net.UDPAddr).Port
			go c.udp(u, "LAN STUN/udp")
		}
	}
	if u, err := net.ListenPacket("udp", "127.0.0.1:0"); err == nil {
		c.STUNLoop = u.LocalAddr().(*net.UDPAddr).Port
		go c.udp(u, "127.0.0.1 STUN/udp")
	}
	return nil
}

func (c *Canaries) udp(u net.PacketConn, name string) {
	buf := make([]byte, 2048)
	for {
		n, from, err := u.ReadFrom(buf)
		if err != nil {
			return
		}
		c.add(name, from.String(), fmt.Sprintf("%d bytes", n))
	}
}

// Site serves the spike's pages.
type Site struct {
	C    *Canaries
	Port int
	// Nonce makes the DNS-prefetch names unique per run.
	Nonce string
}

func (s *Site) Base() string { return fmt.Sprintf("http://%s:%d", siteName, s.Port) }

func (s *Site) Start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.Port = ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc("/net", s.netPage)
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Query().Get("to"), http.StatusFound)
	})
	mux.HandleFunc("/jsnav", func(w http.ResponseWriter, r *http.Request) {
		to, _ := json.Marshal(r.URL.Query().Get("to"))
		fmt.Fprintf(w, "<!doctype html><title>jsnav</title><script>location.href=%s</script>", to)
	})
	mux.HandleFunc("/api/post.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(articleFetch)
	})
	mux.HandleFunc("/api/prices.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(prices())
	})
	mux.HandleFunc("/api/skeleton.json", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(600 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(articleSkeleton)
	})
	mux.HandleFunc("/js/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/js/")
		p, ok := jsPages[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, p.HTML)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><title>upload</title><form><input type=file id=f><a id=dl href="/file.txt" download>download</a></form>`)
	})
	mux.HandleFunc("/file.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=file.txt")
		fmt.Fprint(w, "a file")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<!doctype html><title>spike047 site</title><p>The spike's own site.</p>")
	})
	go http.Serve(ln, mux)
	return nil
}

func (s *Site) netPage(w http.ResponseWriter, r *http.Request) {
	c := s.C
	loop := fmt.Sprintf("127.0.0.1:%d", c.Loop)
	lan := fmt.Sprintf("%s:%d", c.LAN, c.LanTCP)
	nip := fmt.Sprintf("%s.nip.io:%d", c.LAN, c.LanTCP)
	rep := strings.NewReplacer(
		"LOOP", loop, "LOCALHOST", fmt.Sprintf("localhost:%d", c.Loop), "V6", fmt.Sprintf("[::1]:%d", c.Loop),
		"LANADDR", lan, "LANNIP", nip, "LTME", fmt.Sprintf("localtest.me:%d", c.Loop),
		"STUNLAN", fmt.Sprintf("%s:%d", c.LAN, c.STUN), "STUNLOOP", fmt.Sprintf("127.0.0.1:%d", c.STUNLoop),
		"TURNLAN", fmt.Sprintf("%s:%d", c.LAN, c.TURN), "NONCE", s.Nonce, "ZERO", fmt.Sprintf("0.0.0.0:%d", c.Loop),
	)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, rep.Replace(netHTML))
}

const netHTML = `<!doctype html><html><head><title>net probes</title>
<link rel="dns-prefetch" href="//p9-NONCE.example.com">
<link rel="preconnect" href="https://p9b-NONCE.example.org">
</head><body><h1>Network probes</h1><pre id="out"></pre>
<img src="http://LOOP/p1-img-loopback">
<img src="http://LOCALHOST/p1b-img-localhost">
<img src="http://V6/p1c-img-ipv6-loopback">
<img src="http://ZERO/p1e-img-0000">
<img src="http://LANADDR/p2-img-lan">
<iframe src="http://LANADDR/p2b-iframe-lan"></iframe>
<img src="http://LTME/p3-img-public-name-to-loopback">
<img src="http://LANNIP/p3b-img-public-name-to-lan">
<img src="/redirect?to=http://LOOP/p4-redirect-to-loopback">
<img src="/redirect?to=http://LTME/p4b-redirect-to-name">
<a href="https://p9c-NONCE.example.net/">a link</a>
<script>
const out = document.getElementById('out');
const log = (k, v) => { out.textContent += k + ': ' + v + '\n'; };
fetch('http://LOOP/p1d-fetch-loopback', {mode: 'no-cors'}).then(() => log('fetch', 'ok')).catch(e => log('fetch', 'err ' + e));
try { const ws = new WebSocket('ws://LOOP/p5-ws-loopback'); ws.onopen = () => log('ws', 'open'); ws.onerror = () => log('ws', 'error'); } catch (e) { log('ws', 'throw ' + e); }
try { const ws2 = new WebSocket('ws://LANADDR/p5b-ws-lan'); ws2.onopen = () => log('ws2', 'open'); ws2.onerror = () => log('ws2', 'error'); } catch (e) { log('ws2', 'throw ' + e); }
try { const es = new EventSource('http://LOOP/p6-es-loopback'); es.onopen = () => log('es', 'open'); es.onerror = () => { log('es', 'error'); es.close(); }; } catch (e) { log('es', 'throw ' + e); }
try { log('beacon', navigator.sendBeacon('http://LOOP/p7-beacon-loopback', 'x')); } catch (e) { log('beacon', 'throw ' + e); }
try { log('beacon2', navigator.sendBeacon('http://LANADDR/p7b-beacon-lan', 'x')); } catch (e) { log('beacon2', 'throw ' + e); }
try {
  const w = new Worker(URL.createObjectURL(new Blob(["fetch('http://LOOP/p11-worker-fetch',{mode:'no-cors'}).then(()=>postMessage('ok')).catch(e=>postMessage('err '+e))"])));
  w.onmessage = m => log('worker', m.data);
} catch (e) { log('worker', 'throw ' + e); }
(async () => {
  try {
    const pc = new RTCPeerConnection({iceServers: [{urls: 'stun:STUNLAN'}, {urls: 'stun:STUNLOOP'}, {urls: 'turn:TURNLAN?transport=tcp', username: 'u', credential: 'c'}]});
    pc.createDataChannel('x');
    pc.onicecandidate = e => log('rtc', e.candidate ? e.candidate.candidate : 'done');
    await pc.setLocalDescription(await pc.createOffer());
  } catch (e) { log('rtc', 'throw ' + e); }
})();
</script></body></html>`

// ---- JavaScript-only pages ----

type jsPage struct {
	ID      string
	What    string
	HTML    string
	Phrases []string // must be in the readable text
	Hash    string   // opened with this fragment
}

var jsPages = map[string]jsPage{}

func addPage(p jsPage) { jsPages[p.ID] = p }

func pageIDs() []string {
	var ids []string
	for id := range jsPages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

type article struct {
	Title string   `json:"title"`
	Paras []string `json:"paras"`
}

var articleInline = article{"Tidal power in the Severn estuary", []string{
	"The Severn estuary has one of the largest tidal ranges in the world, about fourteen metres at Avonmouth on a spring tide.",
	"Engineers have proposed barrages, lagoons and fences for more than a century; none has been built, mostly because of cost and the harm to mudflats that birds depend on.",
	"A lagoon design walls off part of the estuary and runs turbines in both directions, so it makes power on the incoming and the outgoing tide.",
	"Critics point out that the output follows the moon, not demand, and that two slack periods a day need storage or other plants to cover them.",
	"Supporters answer that the output is predictable decades ahead, which is more than wind or sun can offer, and that the walls last a century or more.",
}}

var articleFetch = article{"Field notes on urban foxes", []string{
	"Red foxes moved into British cities in the 1930s, when suburbs with large gardens spread along new railway lines.",
	"A city fox's territory is small, often less than a square kilometre, because food is everywhere: bins, compost heaps and people who feed them on purpose.",
	"Mange outbreaks can halve a local population within two years, after which foxes from neighbouring territories move in again.",
	"Camera traps show that most urban foxes are active between midnight and four in the morning, when streets are quiet.",
}}

var articleSkeleton = article{"Restoring a 1970s synthesizer", []string{
	"The first job on an old synthesizer is to replace the electrolytic capacitors in the power supply, which dry out after forty years.",
	"Oscillator drift usually comes from tired trimmer potentiometers; cleaning them and re-tuning against a frequency counter fixes most units.",
	"Keyboard contacts collect oxide, so a note that triggers only when pressed hard needs its gold wires cleaned with isopropyl alcohol.",
	"Finally, calibrate the volt-per-octave scaling at both ends of the keyboard, since the two trimmers interact.",
}}

func prices() []map[string]any {
	var rows []map[string]any
	for i, n := range []string{"Alder", "Birch", "Cedar", "Douglas fir", "Elm", "Fir", "Gum", "Hazel", "Iroko", "Juniper", "Kauri", "Larch"} {
		rows = append(rows, map[string]any{"wood": n, "price": 410 + 37*i, "stock": 3 + (i*7)%11})
	}
	return rows
}

const renderJS = `function render(a){const r=document.getElementById('root');r.innerHTML='';const art=document.createElement('article');const h=document.createElement('h1');h.textContent=a.title;art.appendChild(h);for(const p of a.paras){const e=document.createElement('p');e.textContent=p;art.appendChild(e);}r.appendChild(art);document.title=a.title;}`

func shell(title, body, script string) string {
	return "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>" + title + "</title></head><body>" +
		body + "<noscript>You need to enable JavaScript to run this app.</noscript><script>" + renderJS + script + "</script></body></html>"
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func init() {
	addPage(jsPage{ID: "spa-inline", What: "empty root; the article is inline JSON rendered by script",
		HTML:    shell("App", `<div id="root"></div>`, "render("+mustJSON(articleInline)+");"),
		Phrases: []string{"fourteen metres at Avonmouth", "two slack periods a day"}})
	addPage(jsPage{ID: "spa-fetch", What: "empty root; the article comes from a JSON API",
		HTML:    shell("App", `<div id="root"></div>`, "fetch('/api/post.json').then(r=>r.json()).then(render);"),
		Phrases: []string{"square kilometre", "between midnight and four"}})
	addPage(jsPage{ID: "spa-delayed", What: "content appears 2.5 s after load",
		HTML:    shell("App", `<div id="root"></div>`, "setTimeout(()=>render("+mustJSON(articleInline)+"),2500);"),
		Phrases: []string{"fourteen metres at Avonmouth"}})
	addPage(jsPage{ID: "spa-hash", What: "a hash router picks the content from the fragment", Hash: "#/guide/install",
		HTML: shell("Docs", `<div id="root"></div>`, `const routes={'#/guide/install':{title:'Installing the field recorder',paras:[
'Unpack the recorder and charge the battery for at least three hours before the first use, even if the indicator shows full.',
'Insert a card of 32 GB or more, formatted in the recorder itself rather than on a computer, so the folder layout is right.',
'Set the date and time first: every file name carries them, and fixing them later means renaming each recording by hand.',
'Finally update the firmware from the vendor site, since early units shipped with a bug that clipped loud inputs.']}};
function go(){render(routes[location.hash]||{title:'Docs',paras:['Pick a page.']});}window.onhashchange=go;go();`),
		Phrases: []string{"charge the battery for at least three hours", "clipped loud inputs"}})
	addPage(jsPage{ID: "shadow", What: "a web component renders the article inside its shadow root",
		HTML:    shell("Component", `<x-article></x-article>`, `customElements.define('x-article',class extends HTMLElement{connectedCallback(){const s=this.attachShadow({mode:'open'});const a=`+mustJSON(articleInline)+`;s.innerHTML='<article><h1>'+a.title+'</h1>'+a.paras.map(p=>'<p>'+p+'</p>').join('')+'</article>';document.title=a.title;}});`),
		Phrases: []string{"fourteen metres at Avonmouth"}})
	addPage(jsPage{ID: "iframe", What: "the shell loads the article in a same-origin iframe",
		HTML:    shell("Frame", `<div id="root"><iframe src="/js/iframe-inner" style="width:100%;height:600px"></iframe></div>`, ""),
		Phrases: []string{"square kilometre"}})
	addPage(jsPage{ID: "iframe-inner", What: "(the iframe's document)",
		HTML: "<!doctype html><title>inner</title><article><h1>" + articleFetch.Title + "</h1><p>" + strings.Join(articleFetch.Paras, "</p><p>") + "</p></article>"})
	addPage(jsPage{ID: "persian", What: "a Persian article (RTL, ZWNJ) rendered by script",
		HTML: strings.Replace(shell("برنامه", `<div id="root" dir="rtl"></div>`, "render("+mustJSON(article{"باغ‌های ایرانی و قنات", []string{
			"باغ ایرانی بر پایهٔ آب ساخته می‌شود؛ قنات آب را از دامنهٔ کوه زیر زمین می‌آورد و در جوی‌هایی میان درختان پخش می‌کند.",
			"چهارباغ، طرحی است که باغ را با دو محور آب به چهار بخش تقسیم می‌کند و کوشک در میانهٔ آن قرار می‌گیرد.",
			"باغ فین کاشان و باغ شازدهٔ ماهان از نمونه‌های شناخته‌شده‌اند که آب در آن‌ها از پله‌ها و حوض‌ها می‌گذرد.",
			"نگهداری قنات کار دشواری است و مقنی‌ها هر سال چاه‌ها را لایروبی می‌کنند تا آب کم نشود.",
		}})+");"), `lang="en"`, `lang="fa"`, 1),
		Phrases: []string{"دو محور آب به چهار بخش", "مقنی‌ها هر سال"}})
	addPage(jsPage{ID: "table", What: "a price table built from a JSON API",
		HTML:    shell("Timber prices", `<div id="root"><h1>Timber prices</h1></div>`, `fetch('/api/prices.json').then(r=>r.json()).then(rows=>{const t=document.createElement('table');t.innerHTML='<thead><tr><th>Wood</th><th>Price per m³</th><th>Stock</th></tr></thead>';const b=document.createElement('tbody');for(const r of rows){const tr=document.createElement('tr');tr.innerHTML='<td>'+r.wood+'</td><td>'+r.price+' EUR</td><td>'+r.stock+'</td>';b.appendChild(tr);}t.appendChild(b);document.getElementById('root').appendChild(t);const p=document.createElement('p');p.textContent='Prices are for kiln-dried boards, delivered within the region, and change every Monday morning when the sawmill publishes its list.';document.getElementById('root').appendChild(p);});`),
		Phrases: []string{"Douglas fir", "521 EUR", "kiln-dried boards"}})
	addPage(jsPage{ID: "lazy", What: "the first section on load, four more only when scrolled into view",
		HTML: shell("Long read", `<div id="root"></div><div id="more"></div><div id="sentinel" style="height:10px"></div>`, `render({title:'A history of lighthouse lenses',paras:['Before 1820 lighthouses used open fires or oil lamps with polished reflectors, and most of the light went up or sideways instead of out to sea.','Augustin Fresnel cut the lens into concentric rings of glass, which bent the light into one beam with a fraction of the weight of a solid lens.']});
let n=0;const extra=['SECTION TWO: the first-order lens stood more than three metres tall and weighed several tonnes.','SECTION THREE: clockwork turned the lens so each lighthouse flashed its own pattern.','SECTION FOUR: mercury baths let the heavy lens rotate with very little friction.','SECTION FIVE: electric lamps replaced paraffin in the twentieth century.'];
new IntersectionObserver(es=>{if(es[0].isIntersecting&&n<extra.length){for(let i=0;i<2&&n<extra.length;i++){const p=document.createElement('p');p.textContent=extra[n++];document.getElementById('more').appendChild(p);}}}).observe(document.getElementById('sentinel'));
document.getElementById('root').style.minHeight='3000px';`),
		Phrases: []string{"Augustin Fresnel", "SECTION FIVE"}})
	addPage(jsPage{ID: "skeleton", What: "server HTML has skeleton placeholders; script replaces them after an API call",
		HTML:    shell("Loading…", `<div id="root"><div class="skeleton">Loading…</div><div class="skeleton">Loading…</div></div>`, "fetch('/api/skeleton.json').then(r=>r.json()).then(render);"),
		Phrases: []string{"electrolytic capacitors", "volt-per-octave"}})
	addPage(jsPage{ID: "canvas", What: "text drawn only on a canvas, as Excalidraw does",
		HTML: shell("Whiteboard", `<canvas id="c" width="800" height="400"></canvas>`, `const x=document.getElementById('c').getContext('2d');x.font='20px sans-serif';
['Quarterly plan: ship the offline mode first.','Then the export to PDF.','Then the plugin system.'].forEach((t,i)=>x.fillText(t,20,40+30*i));`),
		Phrases: []string{"offline mode"}})
	addPage(jsPage{ID: "consent-overlay", What: "the article, with a consent dialog over it",
		HTML: shell("News", `<div id="root"></div>`, "render("+mustJSON(articleFetch)+`);const d=document.createElement('div');d.setAttribute('role','dialog');d.style='position:fixed;inset:0;background:#fff';
d.innerHTML='<h2>We value your privacy</h2><p>We and our 847 partners store and access information on your device, such as cookies, and process personal data for personalised ads and content, ad and content measurement, audience research and services development.</p><button>Accept all</button><button>Reject all</button><button>Manage options</button>';document.body.appendChild(d);`),
		Phrases: []string{"square kilometre"}})
	addPage(jsPage{ID: "consent-wall", What: "the content loads only after consent is accepted",
		HTML:    shell("News", `<div id="root"></div>`, `document.getElementById('root').innerHTML='<h1>Read on with ads or subscribe</h1><p>To continue reading, accept cookies and tracking by us and our partners, or subscribe to read without ads for 4.99 a month. You can withdraw your consent at any time in the privacy settings at the foot of the page.</p><button id=ok>Accept and continue</button><button>Subscribe</button>';document.getElementById('ok').onclick=()=>render(`+mustJSON(articleFetch)+`);`),
		Phrases: []string{"square kilometre"}})
	addPage(jsPage{ID: "captcha", What: "a bot-check interstitial that never finishes",
		HTML:    shell("Just a moment...", `<div id="root"></div>`, `document.getElementById('root').innerHTML='<h1>example.org</h1><h2>Verify you are human by completing the action below.</h2><div><label><input type=checkbox> I am human</label></div><p>example.org needs to review the security of your connection before proceeding.</p><p>Ray ID: 8f3a2c9d1e7b4a60</p>';`),
		Phrases: []string{"(none)"}})
}

// jsMain lists the pages for section 3 (not the safety pages or the
// iframe's inner document).
var jsMain = []string{"spa-inline", "spa-fetch", "spa-delayed", "spa-hash", "shadow", "iframe", "persian", "table", "lazy", "skeleton", "canvas"}

var jsSafety = []string{"consent-overlay", "consent-wall", "captcha"}
