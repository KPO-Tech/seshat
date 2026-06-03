package web

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	// ActionFetch identifies HTTP or browser-backed page retrieval.
	ActionFetch = "fetch"
	// ActionSearch identifies outbound search requests.
	ActionSearch = "search"
	// ActionMap identifies bounded website discovery requests.
	ActionMap = "map"
	// ActionCrawl identifies bounded multi-page crawl requests.
	ActionCrawl = "crawl"
	// ActionOpen identifies browser page creation.
	ActionOpen = "open"
	// ActionNavigate identifies browser navigation.
	ActionNavigate = "navigate"
	// ActionSnapshot identifies browser read-only extraction.
	ActionSnapshot = "snapshot"
	// ActionClick identifies browser interaction through a click.
	ActionClick = "click"
	// ActionType identifies browser interaction through typing.
	ActionType = "type"
	// ActionPress identifies browser interaction through keyboard presses.
	ActionPress = "press"
	// ActionScroll identifies viewport movement on the current page.
	ActionScroll = "scroll"
	// ActionWait identifies browser stabilization waits.
	ActionWait = "wait"
	// ActionScreenshot identifies visual page capture.
	ActionScreenshot = "screenshot"
	// ActionExtract identifies read-oriented extraction from the current browser page.
	ActionExtract = "extract"
	// ActionListPages identifies page enumeration inside the local browser session.
	ActionListPages = "list_pages"
	// ActionNetworkList identifies read-only browser network inspection.
	ActionNetworkList = "network_list"
	// ActionDownloadList identifies read-only browser download inspection.
	ActionDownloadList = "list_downloads"
	// ActionNetworkPolicy identifies read-only browser network policy inspection.
	ActionNetworkPolicy = "network_policy"
	// ActionSetNetworkPolicy identifies local browser request-policy mutation.
	ActionSetNetworkPolicy = "set_network_policy"
	// ActionSearchContent identifies read-only search over stored browser snapshots.
	ActionSearchContent = "search_content"
	// ActionSelectPage identifies active page switching inside the local browser session.
	ActionSelectPage = "select_page"
	// ActionClosePage identifies page closure inside the local browser session.
	ActionClosePage = "close_page"
)

const (
	// CategoryRead is a non-mutating content retrieval action.
	CategoryRead = "read"
	// CategorySearch is a query-oriented discovery action.
	CategorySearch = "search"
	// CategoryNavigate changes browser location or opens a page.
	CategoryNavigate = "navigate"
	// CategoryInteract performs a user-like page interaction.
	CategoryInteract = "interact"
	// CategorySession mutates only ephemeral local browser session state.
	CategorySession = "session"
)

// PreapprovedHosts enumerates hosts that are safe to access without extra prompts.
var PreapprovedHosts = map[string]bool{
	// ── Dev tools & documentation ──────────────────────────────────────────
	"platform.claude.com":       true,
	"code.claude.com":           true,
	"modelcontextprotocol.io":   true,
	"github.com":                true,
	"gitlab.com":                true,
	"agentskills.io":            true,
	"stackoverflow.com":         true,
	"stackexchange.com":         true,
	"docs.python.org":           true,
	"en.cppreference.com":       true,
	"docs.oracle.com":           true,
	"learn.microsoft.com":       true,
	"developer.mozilla.org":     true,
	"go.dev":                    true,
	"pkg.go.dev":                true,
	"www.php.net":               true,
	"docs.swift.org":            true,
	"kotlinlang.org":            true,
	"ruby-doc.org":              true,
	"doc.rust-lang.org":         true,
	"www.typescriptlang.org":    true,
	"react.dev":                 true,
	"angular.io":                true,
	"vuejs.org":                 true,
	"nextjs.org":                true,
	"expressjs.com":             true,
	"nodejs.org":                true,
	"bun.sh":                    true,
	"jquery.com":                true,
	"getbootstrap.com":          true,
	"tailwindcss.com":           true,
	"d3js.org":                  true,
	"threejs.org":               true,
	"redux.js.org":              true,
	"webpack.js.org":            true,
	"jestjs.io":                 true,
	"reactrouter.com":           true,
	"docs.djangoproject.com":    true,
	"flask.palletsprojects.com": true,
	"fastapi.tiangolo.com":      true,
	"pandas.pydata.org":         true,
	"numpy.org":                 true,
	"www.tensorflow.org":        true,
	"pytorch.org":               true,
	"scikit-learn.org":          true,
	"matplotlib.org":            true,
	"requests.readthedocs.io":   true,
	"laravel.com":               true,
	"symfony.com":               true,
	"wordpress.org":             true,
	"docs.spring.io":            true,
	"hibernate.org":             true,
	"tomcat.apache.org":         true,
	"gradle.org":                true,
	"maven.apache.org":          true,
	"asp.net":                   true,
	"dotnet.microsoft.com":      true,
	"nuget.org":                 true,
	"blazor.net":                true,
	"reactnative.dev":           true,
	"docs.flutter.dev":          true,
	"developer.apple.com":       true,
	"developer.android.com":     true,
	"keras.io":                  true,
	"spark.apache.org":          true,
	"huggingface.co":            true,
	"www.kaggle.com":            true,
	"www.mongodb.com":           true,
	"redis.io":                  true,
	"www.postgresql.org":        true,
	"dev.mysql.com":             true,
	"www.sqlite.org":            true,
	"graphql.org":               true,
	"prisma.io":                 true,
	"docs.aws.amazon.com":       true,
	"cloud.google.com":          true,
	"kubernetes.io":             true,
	"www.docker.com":            true,
	"www.terraform.io":          true,
	"www.ansible.com":           true,
	"vercel.com":                true,
	"docs.netlify.com":          true,
	"cypress.io":                true,
	"selenium.dev":              true,
	"docs.unity.com":            true,
	"docs.unrealengine.com":     true,
	"git-scm.com":               true,
	"nginx.org":                 true,
	"httpd.apache.org":          true,
	"dev.to":                    true,
	"hashnode.com":              true,
	"replit.com":                true,
	"codepen.io":                true,
	"jsfiddle.net":              true,

	// ── Tech & AI companies (blogs, research, newsrooms) ──────────────────
	"openai.com":                  true,
	"anthropic.com":               true,
	"deepmind.google":             true,
	"ai.google":                   true,
	"blog.google":                 true,
	"developers.googleblog.com":   true,
	"research.google":             true,
	"ai.meta.com":                 true,
	"engineering.fb.com":          true,
	"about.meta.com":              true,
	"blogs.microsoft.com":         true,
	"techcommunity.microsoft.com": true,
	"azure.microsoft.com":         true,
	"aws.amazon.com":              true,
	"aboutamazon.com":             true,
	"apple.com":                   true,
	"newsroom.spotify.com":        true,
	"engineering.atspotify.com":   true,
	"netflixtechblog.com":         true,
	"engineering.linkedin.com":    true,
	"blog.linkedin.com":           true,
	"x.com":                       true,
	"blog.x.com":                  true,
	"databricks.com":              true,
	"stripe.com":                  true,
	"shopify.engineering":         true,
	"engineering.shopify.com":     true,
	"mistral.ai":                  true,
	"cohere.com":                  true,
	"stability.ai":                true,
	"nvidia.com":                  true,
	"developer.nvidia.com":        true,
	"intel.com":                   true,
	"amd.com":                     true,
	"arm.com":                     true,
	"qualcomm.com":                true,
	"samsung.com":                 true,
	"ibm.com":                     true,
	"research.ibm.com":            true,
	"oracle.com":                  true,
	"salesforce.com":              true,
	"engineering.salesforce.com":  true,
	"sap.com":                     true,

	// ── Tech news & media ─────────────────────────────────────────────────
	"techcrunch.com":         true,
	"theverge.com":           true,
	"wired.com":              true,
	"arstechnica.com":        true,
	"engadget.com":           true,
	"thenextweb.com":         true,
	"zdnet.com":              true,
	"cnet.com":               true,
	"venturebeat.com":        true,
	"infoq.com":              true,
	"news.ycombinator.com":   true,
	"gizmodo.com":            true,
	"pcmag.com":              true,
	"tomshardware.com":       true,
	"medium.com":             true,
	"substack.com":           true,
	"towardsdatascience.com": true,
	"hackernoon.com":         true,

	// ── International news & press ────────────────────────────────────────
	"reuters.com":        true,
	"apnews.com":         true,
	"bbc.com":            true,
	"bbc.co.uk":          true,
	"theguardian.com":    true,
	"nytimes.com":        true,
	"wsj.com":            true,
	"ft.com":             true,
	"bloomberg.com":      true,
	"economist.com":      true,
	"cnn.com":            true,
	"nbcnews.com":        true,
	"cbsnews.com":        true,
	"axios.com":          true,
	"politico.com":       true,
	"theatlantic.com":    true,
	"time.com":           true,
	"newsweek.com":       true,
	"usatoday.com":       true,
	"washingtonpost.com": true,
	"latimes.com":        true,
	"euronews.com":       true,
	"france24.com":       true,
	"rfi.fr":             true,
	"dw.com":             true,
	"aljazeera.com":      true,
	"nhk.or.jp":          true,
	"scmp.com":           true,
	"thehindu.com":       true,

	// ── French press ──────────────────────────────────────────────────────
	"lemonde.fr":        true,
	"lefigaro.fr":       true,
	"lesechos.fr":       true,
	"latribune.fr":      true,
	"liberation.fr":     true,
	"20minutes.fr":      true,
	"bfmtv.com":         true,
	"leparisien.fr":     true,
	"lopinion.fr":       true,
	"challenges.fr":     true,
	"usine-digitale.fr": true,
	"01net.com":         true,
	"journaldunet.com":  true,
	"numerama.com":      true,
	"frenchweb.fr":      true,
	"maddyness.com":     true,

	// ── Finance & global markets ──────────────────────────────────────────
	"finance.yahoo.com":       true,
	"marketwatch.com":         true,
	"investing.com":           true,
	"seekingalpha.com":        true,
	"morningstar.com":         true,
	"fool.com":                true,
	"cnbc.com":                true,
	"businessinsider.com":     true,
	"tradingview.com":         true,
	"nasdaq.com":              true,
	"nyse.com":                true,
	"euronext.com":            true,
	"londonstockexchange.com": true,
	"boerse-frankfurt.de":     true,
	"coindesk.com":            true,
	"coinmarketcap.com":       true,
	"cointelegraph.com":       true,
	"cryptonews.com":          true,
	"sec.gov":                 true,
	"federalreserve.gov":      true,
	"ecb.europa.eu":           true,
	"imf.org":                 true,
	"worldbank.org":           true,
	"bis.org":                 true,
	"bls.gov":                 true,
	"oecd.org":                true,
	"stats.oecd.org":          true,
	"statista.com":            true,
	"data.worldbank.org":      true,
	"macrotrends.net":         true,
	"multpl.com":              true,
	"wisesheets.io":           true,

	// ── Business, entrepreneurship & consulting ───────────────────────────
	"hbr.org":             true,
	"mit.edu":             true,
	"stanford.edu":        true,
	"harvard.edu":         true,
	"ycombinator.com":     true,
	"producthunt.com":     true,
	"crunchbase.com":      true,
	"pitchbook.com":       true,
	"angel.co":            true,
	"sifted.eu":           true,
	"entrepreneur.com":    true,
	"inc.com":             true,
	"fastcompany.com":     true,
	"forbes.com":          true,
	"fortune.com":         true,
	"mckinsey.com":        true,
	"bcg.com":             true,
	"bain.com":            true,
	"deloitte.com":        true,
	"pwc.com":             true,
	"kpmg.com":            true,
	"ey.com":              true,
	"accenture.com":       true,
	"a16z.com":            true,
	"sequoiacap.com":      true,
	"indexventures.com":   true,
	"first.org":           true,
	"seedtable.com":       true,
	"lesdecodeurs.fr":     true,
	"bpifrance.fr":        true,
	"lafrenchtech.com":    true,
	"entreprises.gouv.fr": true,

	// ── Official & international institutions ─────────────────────────────
	"europa.eu":           true,
	"ec.europa.eu":        true,
	"europarl.europa.eu":  true,
	"gov.uk":              true,
	"gouv.fr":             true,
	"service-public.fr":   true,
	"legifrance.gouv.fr":  true,
	"data.gouv.fr":        true,
	"insee.fr":            true,
	"canada.ca":           true,
	"whitehouse.gov":      true,
	"congress.gov":        true,
	"un.org":              true,
	"who.int":             true,
	"wto.org":             true,
	"weforum.org":         true,
	"nato.int":            true,
	"consilium.europa.eu": true,
	"afd.fr":              true,
	"banque-france.fr":    true,
	"amf-france.org":      true,

	// ── Research & academia ───────────────────────────────────────────────
	"arxiv.org":               true,
	"pubmed.ncbi.nlm.nih.gov": true,
	"nature.com":              true,
	"science.org":             true,
	"cell.com":                true,
	"ieee.org":                true,
	"acm.org":                 true,
	"ssrn.com":                true,
	"jstor.org":               true,
	"researchgate.net":        true,
	"semanticscholar.org":     true,
	"scholar.google.com":      true,
	"openreview.net":          true,
	"distill.pub":             true,
	"papers.nips.cc":          true,
	"proceedings.mlr.press":   true,

	// ── Market intelligence & analytics ──────────────────────────────────
	"gartner.com":    true,
	"forrester.com":  true,
	"idc.com":        true,
	"nielsen.com":    true,
	"kantar.com":     true,
	"similarweb.com": true,
	"semrush.com":    true,
	"ahrefs.com":     true,
	"moz.com":        true,
	"sprinklr.com":   true,
	"brandwatch.com": true,

	// ── Africa — news, press & media ─────────────────────────────────────
	"allafrica.com":               true,
	"theafricareport.com":         true,
	"africanbusinessmagazine.com": true,
	"howwemadeitinafrica.com":     true,
	"jeuneafrique.com":            true,
	"africanews.com":              true,
	"africa24.com":                true,
	"quarzafrica.com":             true,
	"qz.com":                      true,
	// West Africa
	"businessday.ng":        true,
	"punchng.com":           true,
	"guardian.ng":           true,
	"vanguardngr.com":       true,
	"premiumtimesng.com":    true,
	"thenationonlineng.net": true,
	"dailytrust.com":        true,
	"ghanaiantimes.com.gh":  true,
	"graphic.com.gh":        true,
	"myjoyonline.com":       true,
	"pulse.com.gh":          true,
	"pulse.ng":              true,
	"abidjan.net":           true,
	"koaci.com":             true,
	"seneweb.com":           true,
	"dakaractu.com":         true,
	"maliactu.net":          true,
	"beninwebtv.com":        true,
	// East Africa
	"nation.africa":        true,
	"theeastafrican.co.ke": true,
	"standardmedia.co.ke":  true,
	"monitor.co.ug":        true,
	"thecitizen.co.tz":     true,
	"newtimes.co.rw":       true,
	"capitalfm.co.ke":      true,
	// Southern Africa
	"dailymaverick.co.za": true,
	"mg.co.za":            true,
	"news24.com":          true,
	"businesslive.co.za":  true,
	"fin24.com":           true,
	"moneyweb.co.za":      true,
	"heraldlive.co.za":    true,
	// North Africa
	"ahram.org.eg":    true,
	"egypttoday.com":  true,
	"lematin.ma":      true,
	"hespress.com":    true,
	"tsa-algerie.com": true,
	"tap.info.tn":     true,
	// Pan-African tech & business
	"techcabal.com":                 true,
	"techpoint.africa":              true,
	"disrupt-africa.com":            true,
	"disruptafrica.com":             true, // alias — same site, both spellings in use
	"venturesafrica.com":            true,
	"africabusinesscommunities.com": true,
	"itnewsafrica.com":              true,
	"apanews.net":                   true,
	"cgtnews.com":                   true, // CGTN Africa

	// ── Africa — markets, institutions & finance ──────────────────────────
	"afdb.org":               true, // African Development Bank
	"au.int":                 true, // African Union
	"uneca.org":              true, // UN Economic Commission for Africa
	"afreximbank.com":        true,
	"atidi.org":              true, // African Trade Insurance Agency
	"ngxgroup.com":           true, // Nigerian Exchange Group
	"nse.co.ke":              true, // Nairobi Securities Exchange
	"jse.co.za":              true, // Johannesburg Stock Exchange
	"brvm.org":               true, // Bourse Régionale UEMOA
	"casablanca-bourse.com":  true, // Casablanca Stock Exchange
	"egx.com.eg":             true, // Egyptian Exchange
	"gse.com.gh":             true, // Ghana Stock Exchange
	"dse.co.tz":              true, // Dar es Salaam Stock Exchange
	"use.or.ug":              true, // Uganda Securities Exchange
	"africainvestment.net":   true,
	"africafinancegroup.com": true,
	"mospi.co.za":            true,
	"ifc.org":                true, // IFC / World Bank Group Africa
	"miga.org":               true,
	"businessafrica.net":     true,
	"africaninvestor.com":    true,
	// African fintech & payments
	"flutterwave.com": true,
	"paystack.com":    true,
	"safaricom.co.ke": true, // M-Pesa
	"mtn.com":         true,
	"orange.com":      true,
	"andela.com":      true,
	"jumia.com":       true,

	// ── Investments ───────────────────────────────────────────────────────
	"investopedia.com":       true,
	"fidelity.com":           true,
	"schwab.com":             true,
	"interactivebrokers.com": true,
	"tdameritrade.com":       true,
	"etrade.com":             true,
	"robinhood.com":          true,
	"webull.com":             true,
	"vanguard.com":           true,
	"blackrock.com":          true,
	"pimco.com":              true,
	"berkshirehathaway.com":  true,
	"cbinsights.com":         true,
	"preqin.com":             true,
	"hfr.com":                true,
	"cmegroup.com":           true, // CME / NYMEX / COMEX futures
	"theice.com":             true, // ICE — NYSE, energy markets
	"lme.com":                true, // London Metal Exchange
	"etf.com":                true,
	"etfdb.com":              true,
	"treasurydirect.gov":     true,
	"cfainstitute.org":       true,
	"world-exchanges.org":    true,
	"finviz.com":             true,
	"smartasset.com":         true,
	"nerdwallet.com":         true,
	"bankrate.com":           true,
	"kiplinger.com":          true,

	// ── Video, community & reference ─────────────────────────────────────
	"youtube.com":        true,
	"youtu.be":           true,
	"vimeo.com":          true,
	"twitch.tv":          true,
	"reddit.com":         true,
	"quora.com":          true,
	"wikipedia.org":      true,
	"wikimedia.org":      true,
	"wikidata.org":       true,
	"archive.org":        true,
	"slideshare.net":     true,
	"scribd.com":         true,
	"goodreads.com":      true,
	"podcasts.apple.com": true,
	"open.spotify.com":   true,
	"ted.com":            true,
	"coursera.org":       true,
	"edx.org":            true,
	"khanacademy.org":    true,
	"udemy.com":          true,
	"pluralsight.com":    true,
}

// DomainCategory groups related domains for the frontend domain catalog UI.
type DomainCategory struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Icon    string   `json:"icon"`
	Domains []string `json:"domains"`
}

// DomainCatalog returns the curated pre-approved domain list organized by category.
func DomainCatalog() []DomainCategory {
	return []DomainCategory{
		{
			ID:    "tech_ai",
			Label: "Tech & AI",
			Icon:  "robot",
			Domains: []string{
				"openai.com", "anthropic.com", "deepmind.google", "ai.google",
				"blog.google", "developers.googleblog.com", "research.google",
				"ai.meta.com", "engineering.fb.com", "blogs.microsoft.com",
				"azure.microsoft.com", "aws.amazon.com", "aboutamazon.com",
				"apple.com", "netflixtechblog.com", "engineering.atspotify.com",
				"engineering.linkedin.com", "databricks.com", "stripe.com",
				"shopify.engineering", "mistral.ai", "cohere.com", "stability.ai",
				"nvidia.com", "ibm.com", "research.ibm.com", "salesforce.com",
				"oracle.com", "sap.com",
			},
		},
		{
			ID:    "tech_news",
			Label: "Tech News",
			Icon:  "news",
			Domains: []string{
				"techcrunch.com", "theverge.com", "wired.com", "arstechnica.com",
				"engadget.com", "thenextweb.com", "zdnet.com", "cnet.com",
				"venturebeat.com", "infoq.com", "news.ycombinator.com",
				"gizmodo.com", "medium.com", "substack.com", "hackernoon.com",
				"towardsdatascience.com", "dev.to", "pcmag.com", "tomshardware.com",
			},
		},
		{
			ID:    "world_news",
			Label: "World News",
			Icon:  "newspaper",
			Domains: []string{
				"reuters.com", "apnews.com", "bbc.com", "theguardian.com",
				"nytimes.com", "wsj.com", "ft.com", "bloomberg.com",
				"economist.com", "cnn.com", "axios.com", "theatlantic.com",
				"time.com", "washingtonpost.com", "euronews.com", "france24.com",
				"aljazeera.com", "dw.com", "rfi.fr", "scmp.com",
			},
		},
		{
			ID:    "french_press",
			Label: "Presse Française",
			Icon:  "france",
			Domains: []string{
				"lemonde.fr", "lefigaro.fr", "lesechos.fr", "latribune.fr",
				"liberation.fr", "bfmtv.com", "leparisien.fr", "challenges.fr",
				"usine-digitale.fr", "journaldunet.com", "numerama.com",
				"frenchweb.fr", "maddyness.com", "01net.com",
			},
		},
		{
			ID:    "finance_markets",
			Label: "Finance & Markets",
			Icon:  "chart",
			Domains: []string{
				"bloomberg.com", "ft.com", "wsj.com", "cnbc.com",
				"marketwatch.com", "investing.com", "morningstar.com",
				"seekingalpha.com", "tradingview.com", "nasdaq.com",
				"nyse.com", "euronext.com", "statista.com", "macrotrends.net",
				"coindesk.com", "coinmarketcap.com", "cointelegraph.com",
				"businessinsider.com", "finance.yahoo.com", "fool.com",
			},
		},
		{
			ID:    "official_institutions",
			Label: "Official & Institutions",
			Icon:  "building",
			Domains: []string{
				"imf.org", "worldbank.org", "oecd.org", "weforum.org",
				"un.org", "who.int", "wto.org", "bis.org",
				"ecb.europa.eu", "federalreserve.gov", "sec.gov", "bls.gov",
				"europa.eu", "ec.europa.eu", "gov.uk", "gouv.fr",
				"data.gouv.fr", "insee.fr", "banque-france.fr", "amf-france.org",
				"service-public.fr", "legifrance.gouv.fr", "nato.int",
			},
		},
		{
			ID:    "business_entrepreneurship",
			Label: "Business & Entrepreneurship",
			Icon:  "rocket",
			Domains: []string{
				"hbr.org", "mckinsey.com", "bcg.com", "bain.com",
				"deloitte.com", "pwc.com", "forbes.com", "fortune.com",
				"fastcompany.com", "inc.com", "entrepreneur.com",
				"ycombinator.com", "producthunt.com", "crunchbase.com",
				"a16z.com", "sequoiacap.com", "sifted.eu", "seedtable.com",
				"bpifrance.fr", "lafrenchtech.com", "maddyness.com",
				"pitchbook.com", "angel.co",
			},
		},
		{
			ID:    "research_academia",
			Label: "Research & Academia",
			Icon:  "graduation",
			Domains: []string{
				"arxiv.org", "nature.com", "science.org", "ieee.org",
				"acm.org", "pubmed.ncbi.nlm.nih.gov", "researchgate.net",
				"semanticscholar.org", "scholar.google.com", "openreview.net",
				"distill.pub", "ssrn.com", "jstor.org",
				"mit.edu", "stanford.edu", "harvard.edu",
			},
		},
		{
			ID:    "market_intelligence",
			Label: "Market Intelligence",
			Icon:  "analytics",
			Domains: []string{
				"gartner.com", "forrester.com", "idc.com", "nielsen.com",
				"kantar.com", "statista.com", "similarweb.com",
				"semrush.com", "ahrefs.com", "brandwatch.com",
				"crunchbase.com", "pitchbook.com",
			},
		},
		{
			ID:    "africa_news",
			Label: "Africa — News & Press",
			Icon:  "africa",
			Domains: []string{
				// Pan-African
				"allafrica.com", "theafricareport.com", "africanbusinessmagazine.com",
				"howwemadeitinafrica.com", "jeuneafrique.com", "africanews.com",
				"qz.com", "apanews.net", "cgtnews.com",
				// West Africa
				"businessday.ng", "punchng.com", "guardian.ng", "premiumtimesng.com",
				"pulse.ng", "myjoyonline.com", "graphic.com.gh",
				"abidjan.net", "seneweb.com", "dakaractu.com",
				// East Africa
				"nation.africa", "theeastafrican.co.ke", "standardmedia.co.ke",
				"monitor.co.ug", "thecitizen.co.tz", "newtimes.co.rw",
				// Southern Africa
				"dailymaverick.co.za", "mg.co.za", "news24.com",
				"businesslive.co.za", "fin24.com", "moneyweb.co.za",
				// North Africa
				"ahram.org.eg", "egypttoday.com", "lematin.ma", "hespress.com",
				// Tech & business
				"techcabal.com", "techpoint.africa", "disrupt-africa.com", "disruptafrica.com",
				"venturesafrica.com", "itnewsafrica.com",
			},
		},
		{
			ID:    "africa_markets",
			Label: "Africa — Markets & Finance",
			Icon:  "africa-chart",
			Domains: []string{
				// Institutions
				"afdb.org", "au.int", "uneca.org", "afreximbank.com", "ifc.org",
				// Stock exchanges
				"ngxgroup.com", "jse.co.za", "nse.co.ke", "brvm.org",
				"casablanca-bourse.com", "egx.com.eg", "gse.com.gh",
				"dse.co.tz", "use.or.ug",
				// Finance & fintech
				"fin24.com", "moneyweb.co.za", "businesslive.co.za",
				"flutterwave.com", "paystack.com", "safaricom.co.ke",
				"jumia.com", "andela.com",
			},
		},
		{
			ID:    "investments",
			Label: "Investments",
			Icon:  "investment",
			Domains: []string{
				// Education & research
				"investopedia.com", "morningstar.com", "seekingalpha.com",
				"fool.com", "cfainstitute.org", "kiplinger.com",
				// Brokers & platforms
				"fidelity.com", "schwab.com", "vanguard.com",
				"interactivebrokers.com", "etrade.com", "robinhood.com",
				"webull.com", "tdameritrade.com",
				// Asset managers & funds
				"blackrock.com", "pimco.com", "berkshirehathaway.com",
				"preqin.com", "hfr.com",
				// Exchanges & derivatives
				"cmegroup.com", "theice.com", "lme.com",
				"nasdaq.com", "nyse.com", "euronext.com",
				// ETFs & data
				"etf.com", "etfdb.com", "finviz.com", "macrotrends.net",
				"tradingview.com", "investing.com",
				// VC & private
				"cbinsights.com", "crunchbase.com", "pitchbook.com", "angel.co",
				// Regulators
				"sec.gov", "treasurydirect.gov", "world-exchanges.org",
			},
		},
		{
			ID:    "video_community",
			Label: "Video & Community",
			Icon:  "video",
			Domains: []string{
				"youtube.com", "youtu.be", "vimeo.com", "twitch.tv",
				"ted.com", "reddit.com", "quora.com",
				"stackoverflow.com", "github.com", "huggingface.co",
				"wikipedia.org", "archive.org",
			},
		},
		{
			ID:    "learning",
			Label: "Learning & Courses",
			Icon:  "book",
			Domains: []string{
				"coursera.org", "edx.org", "khanacademy.org", "udemy.com",
				"pluralsight.com", "mit.edu", "stanford.edu",
				"developer.mozilla.org", "learn.microsoft.com", "go.dev",
			},
		},
	}
}

// PreapprovedPathPrefixes contains host/path combinations that are allowed more narrowly than the full host.
var PreapprovedPathPrefixes = map[string][]string{
	"github.com": {"/anthropics"},
}

// NormalizeHost strips superficial hostname differences before policy checks.
func NormalizeHost(host string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
}

// IsPreapproved reports whether the hostname is globally preapproved.
func IsPreapproved(hostname string) bool {
	host := NormalizeHost(hostname)
	return PreapprovedHosts[host] || PreapprovedHosts["www."+host]
}

// IsPreapprovedPath reports whether a hostname/path pair is preapproved by either host-wide or path-scoped rules.
func IsPreapprovedPath(hostname, pathname string) bool {
	host := NormalizeHost(hostname)
	if IsPreapproved(host) {
		return true
	}

	prefixes, ok := PreapprovedPathPrefixes[host]
	if !ok {
		return false
	}
	for _, prefix := range prefixes {
		if pathname == prefix || strings.HasPrefix(pathname, prefix+"/") {
			return true
		}
	}
	return false
}

// HostMatchesDomain centralizes host/domain matching for filtering and permission logic.
func HostMatchesDomain(host string, domain string) bool {
	host = NormalizeHost(host)
	domain = NormalizeHost(domain)
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// EnrichFetchPermissionInput normalizes fetch tool inputs into a stable permission shape.
func EnrichFetchPermissionInput(input map[string]any) map[string]any {
	enriched := clonePermissionInput(input, 6)
	enriched["action"] = ActionFetch
	enriched["permission_category"] = CategoryRead
	enriched["resource_kind"] = "url"
	applyURLFields(enriched, readOptionalString(input, "url"))
	if mode := strings.TrimSpace(strings.ToLower(readOptionalString(input, "render_mode"))); mode != "" {
		enriched["render_mode"] = mode
	}
	return enriched
}

// EnrichSearchPermissionInput normalizes search tool inputs into a stable permission shape.
func EnrichSearchPermissionInput(input map[string]any) map[string]any {
	enriched := clonePermissionInput(input, 7)
	enriched["action"] = ActionSearch
	enriched["permission_category"] = CategorySearch
	enriched["resource_kind"] = "search"
	enriched["query"] = strings.TrimSpace(readOptionalString(input, "query"))

	allowed := normalizeDomains(readStringSlice(input["allowed_domains"]))
	blocked := normalizeDomains(readStringSlice(input["blocked_domains"]))
	if len(allowed) > 0 {
		enriched["allowed_domains"] = allowed
	}
	if len(blocked) > 0 {
		enriched["blocked_domains"] = blocked
	}
	if providerMode := strings.TrimSpace(strings.ToLower(readOptionalString(input, "provider_mode"))); providerMode != "" {
		enriched["provider_mode"] = providerMode
	}
	return enriched
}

// EnrichBrowserPermissionInput normalizes browser tool inputs and optionally attaches the current page URL.
func EnrichBrowserPermissionInput(input map[string]any, action string, currentURL string) map[string]any {
	enriched := clonePermissionInput(input, 8)
	normalizedAction := strings.TrimSpace(strings.ToLower(action))
	enriched["action"] = normalizedAction
	enriched["permission_category"] = categoryForAction(normalizedAction)
	enriched["resource_kind"] = "browser"

	rawURL := readOptionalString(input, "url")
	if rawURL != "" {
		applyURLFields(enriched, rawURL)
	}
	if currentURL = strings.TrimSpace(currentURL); currentURL != "" {
		enriched["current_url"] = currentURL
		applyCurrentURLFields(enriched, currentURL)
		if rawURL == "" {
			enriched["url"] = currentURL
		}
	}
	return enriched
}

// EvaluatePermission applies lightweight shared web policy before the global permission pipeline runs.
func EvaluatePermission(input map[string]any) types.PermissionResult {
	action := strings.TrimSpace(strings.ToLower(readOptionalString(input, "action")))
	category := strings.TrimSpace(strings.ToLower(readOptionalString(input, "permission_category")))
	executionMode := strings.TrimSpace(strings.ToLower(readOptionalString(input, "execution_mode")))

	switch category {
	case CategorySession:
		return types.AllowWithInput("browser session action is local to the current session", input)
	}

	switch category {
	case CategorySearch:
		if domainsArePreapproved(readStringSlice(input["allowed_domains"])) {
			return types.AllowWithInput("search is constrained to preapproved domains", input)
		}
		return types.Passthrough(input)
	case CategoryRead:
		if IsPreapprovedPath(readOptionalString(input, "host"), readOptionalString(input, "path")) {
			return types.AllowWithInput("fetch targets a preapproved web domain", input)
		}
		return types.Passthrough(input)
	case CategoryNavigate:
		if action == ActionOpen && strings.TrimSpace(readOptionalString(input, "url")) == "" {
			return types.AllowWithInput("opening a blank browser page stays local to the current session", input)
		}
		if IsPreapprovedPath(readOptionalString(input, "host"), readOptionalString(input, "path")) {
			return types.AllowWithInput("browser access targets a preapproved web domain", input)
		}
		if IsPreapprovedPath(readOptionalString(input, "current_host"), readOptionalString(input, "current_path")) {
			return types.AllowWithInput("browser action stays on a preapproved page", input)
		}
		if executionMode == "browse" && action == ActionSnapshot {
			return types.AllowWithInput("browse mode allows page inspection within the current browser session", input)
		}
		return types.Passthrough(input)
	case CategoryInteract:
		if executionMode == "browse" && IsPreapprovedPath(readOptionalString(input, "current_host"), readOptionalString(input, "current_path")) {
			return types.Passthrough(input)
		}
		return types.Passthrough(input)
	default:
		return types.Passthrough(input)
	}
}

// PermissionMatcher compiles a shared matcher used by content-specific permission rules.
func PermissionMatcher(input map[string]any) func(string) bool {
	action := readOptionalString(input, "action")
	rawURL := readOptionalString(input, "url")
	currentURL := readOptionalString(input, "current_url")
	host := readOptionalString(input, "host")
	currentHost := readOptionalString(input, "current_host")
	scheme := readOptionalString(input, "scheme")
	renderMode := readOptionalString(input, "render_mode")
	providerMode := readOptionalString(input, "provider_mode")
	allowedDomains := normalizeDomains(readStringSlice(input["allowed_domains"]))
	blockedDomains := normalizeDomains(readStringSlice(input["blocked_domains"]))

	return func(ruleContent string) bool {
		ruleContent = strings.TrimSpace(ruleContent)
		switch {
		case strings.HasPrefix(ruleContent, "action:"):
			return strings.EqualFold(strings.TrimPrefix(ruleContent, "action:"), action)
		case strings.HasPrefix(ruleContent, "host:"):
			value := NormalizeHost(strings.TrimPrefix(ruleContent, "host:"))
			return value != "" && (value == NormalizeHost(host) || value == NormalizeHost(currentHost))
		case strings.HasPrefix(ruleContent, "scheme:"):
			return strings.EqualFold(strings.TrimPrefix(ruleContent, "scheme:"), scheme)
		case strings.HasPrefix(ruleContent, "url:"):
			pattern := strings.TrimPrefix(ruleContent, "url:")
			return wildcardMatch(pattern, rawURL) || wildcardMatch(pattern, currentURL)
		case strings.HasPrefix(ruleContent, "render_mode:"):
			return strings.EqualFold(strings.TrimPrefix(ruleContent, "render_mode:"), renderMode)
		case strings.HasPrefix(ruleContent, "provider_mode:"):
			return strings.EqualFold(strings.TrimPrefix(ruleContent, "provider_mode:"), providerMode)
		case strings.HasPrefix(ruleContent, "allowed_domain:"):
			return containsMatchingDomain(allowedDomains, strings.TrimPrefix(ruleContent, "allowed_domain:"))
		case strings.HasPrefix(ruleContent, "blocked_domain:"):
			return containsMatchingDomain(blockedDomains, strings.TrimPrefix(ruleContent, "blocked_domain:"))
		default:
			return wildcardMatch(ruleContent, rawURL) ||
				wildcardMatch(ruleContent, currentURL) ||
				strings.EqualFold(ruleContent, action) ||
				strings.EqualFold(ruleContent, host) ||
				strings.EqualFold(ruleContent, currentHost)
		}
	}
}

func clonePermissionInput(input map[string]any, extra int) map[string]any {
	enriched := make(map[string]any, len(input)+extra)
	for key, value := range input {
		enriched[key] = value
	}
	return enriched
}

func applyURLFields(target map[string]any, rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}
	target["url"] = rawURL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	target["host"] = NormalizeHost(parsed.Hostname())
	target["scheme"] = strings.ToLower(strings.TrimSpace(parsed.Scheme))
	target["path"] = parsed.EscapedPath()
}

func applyCurrentURLFields(target map[string]any, rawURL string) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return
	}
	target["current_host"] = NormalizeHost(parsed.Hostname())
	target["current_scheme"] = strings.ToLower(strings.TrimSpace(parsed.Scheme))
	target["current_path"] = parsed.EscapedPath()
}

func domainsArePreapproved(domains []string) bool {
	if len(domains) == 0 {
		return false
	}
	for _, domain := range domains {
		if !IsPreapproved(domain) {
			return false
		}
	}
	return true
}

func containsMatchingDomain(domains []string, candidate string) bool {
	candidate = NormalizeHost(candidate)
	if candidate == "" {
		return false
	}
	for _, domain := range domains {
		if HostMatchesDomain(domain, candidate) || HostMatchesDomain(candidate, domain) {
			return true
		}
	}
	return false
}

func normalizeDomains(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		if normalized := NormalizeHost(domain); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func categoryForAction(action string) string {
	switch action {
	case ActionSearch:
		return CategorySearch
	case ActionFetch, ActionMap, ActionCrawl, ActionSnapshot, ActionScreenshot, ActionExtract:
		return CategoryRead
	case ActionOpen, ActionNavigate:
		return CategoryNavigate
	case ActionClick, ActionType, ActionPress:
		return CategoryInteract
	case ActionScroll, ActionWait, ActionListPages, ActionSelectPage, ActionClosePage, ActionSetNetworkPolicy:
		return CategorySession
	case ActionNetworkList, ActionDownloadList, ActionNetworkPolicy, ActionSearchContent:
		return CategoryRead
	default:
		return ""
	}
}

func readOptionalString(value any, keys ...string) string {
	if len(keys) == 0 {
		if s, ok := value.(string); ok {
			return strings.TrimSpace(s)
		}
		return ""
	}

	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func readStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				result = append(result, strings.TrimSpace(s))
			}
		}
		return result
	default:
		return nil
	}
}

func wildcardMatch(pattern string, candidate string) bool {
	pattern = strings.TrimSpace(pattern)
	candidate = strings.TrimSpace(candidate)
	if pattern == "" || candidate == "" {
		return false
	}
	if strings.Contains(pattern, "*") && strings.HasSuffix(pattern, "*") && strings.Count(pattern, "*") == 1 && !strings.Contains(pattern, "?") {
		return strings.HasPrefix(candidate, strings.TrimSuffix(pattern, "*"))
	}
	if strings.ContainsAny(pattern, "*?") {
		matched, err := filepath.Match(pattern, candidate)
		if err == nil {
			return matched
		}
	}
	return strings.EqualFold(pattern, candidate)
}
