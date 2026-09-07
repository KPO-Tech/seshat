module github.com/KPO-Tech/seshat

go 1.26.6

require (
	charm.land/bubbles/v2 v2.1.1
	charm.land/bubbletea/v2 v2.0.8
	charm.land/catwalk v0.51.6
	charm.land/glamour/v2 v2.0.1
	charm.land/lipgloss/v2 v2.0.5
	github.com/Knetic/govaluate v3.0.0+incompatible
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/PuerkitoBio/goquery v1.12.0
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/atotto/clipboard v0.1.4
	github.com/aws/aws-sdk-go-v2 v1.43.6
	github.com/aws/aws-sdk-go-v2/config v1.32.37
	github.com/aws/aws-sdk-go-v2/credentials v1.19.36
	github.com/aws/aws-sdk-go-v2/service/s3 v1.106.1
	github.com/aws/smithy-go v1.27.8
	github.com/aymanbagabas/go-nativeclipboard v0.1.3
	github.com/aymanbagabas/go-udiff v0.4.1
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/charlievieth/fastwalk v1.0.14
	github.com/charmbracelet/colorprofile v0.4.3
	github.com/charmbracelet/ultraviolet v0.0.0-20260703014108-f5a850f9c2b7
	github.com/charmbracelet/x/ansi v0.11.7
	github.com/charmbracelet/x/editor v0.2.0
	github.com/charmbracelet/x/etag v0.2.0
	github.com/charmbracelet/x/exp/charmtone v0.0.0-20260608090822-c3ad58c6c9e5
	github.com/charmbracelet/x/exp/ordered v0.1.0
	github.com/charmbracelet/x/exp/slice v0.0.0-20250904123553-b4e2667e5ad5
	github.com/charmbracelet/x/exp/strings v0.1.0
	github.com/charmbracelet/x/powernap v0.1.6
	github.com/charmbracelet/x/term v0.2.2
	github.com/clipperhouse/displaywidth v0.11.0
	github.com/clipperhouse/uax29/v2 v2.7.0
	github.com/coder/hnsw v0.6.1
	github.com/denisbrodbeck/machineid v1.0.1
	github.com/disintegration/imaging v1.6.2
	github.com/dop251/goja v0.0.0-20260901132549-43234fa61381
	github.com/dustin/go-humanize v1.0.1
	github.com/gen2brain/beeep v0.11.2
	github.com/glebarez/sqlite v1.11.0
	github.com/go-git/go-git/v5 v5.19.2
	github.com/go-pdf/fpdf v0.9.0
	github.com/go-rod/rod v0.116.2
	github.com/go-shiori/go-readability v0.0.0-20251205110129-5db1dc9836f0
	github.com/go-sql-driver/mysql v1.10.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/invopop/jsonschema v0.14.0
	github.com/itchyny/gojq v0.12.19
	github.com/jordanella/go-ansi-paintbrush v0.0.0-20240728195301-b7ad996ecf3d
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728
	github.com/lib/pq v1.12.3
	github.com/lucasb-eyer/go-colorful v1.4.0
	github.com/mattn/go-isatty v0.0.24
	github.com/modelcontextprotocol/go-sdk v1.7.0
	github.com/opensearch-project/opensearch-go/v5 v5.0.0-rc6
	github.com/pdfcpu/pdfcpu v0.13.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/posthog/posthog-go v1.15.0
	github.com/qdrant/go-client v1.18.3
	github.com/qjebbs/go-jsons v1.0.0-alpha.5
	github.com/redis/go-redis/v9 v9.22.0
	github.com/rivo/uniseg v0.4.7
	github.com/robfig/cron/v3 v3.0.1
	github.com/sahilm/fuzzy v0.1.3
	github.com/shopspring/decimal v1.4.0
	github.com/slack-go/slack v0.26.0
	github.com/sourcegraph/jsonrpc2 v0.2.1
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.12.0
	github.com/tidwall/gjson v1.19.0
	github.com/tidwall/sjson v1.2.5
	github.com/xuri/excelize/v2 v2.11.0
	github.com/zeebo/xxh3 v1.1.0
	go.mongodb.org/mongo-driver/v2 v2.8.2
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
	golang.org/x/net v0.57.0
	golang.org/x/sys v0.47.0
	golang.org/x/text v0.41.0
	gonum.org/v1/gonum v0.17.0
	google.golang.org/grpc v1.82.1
	google.golang.org/protobuf v1.36.11
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/mysql v1.6.0
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.31.2
	modernc.org/sqlite v1.57.0
	mvdan.cc/sh/v3 v3.13.1
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	git.sr.ht/~jackmordaunt/go-toast v1.1.2 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.15 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.37 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.33 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.6 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/chewxy/math32 v1.10.1 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/ebitengine/purego v0.10.1 // indirect
	github.com/esiqveland/notify v0.13.3 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/glebarez/go-sqlite v1.21.2 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-shiori/dom v0.0.0-20230515143342-73569d674e1c // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogs/chardet v0.0.0-20211120154057-b7413eaefb8f // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3 // indirect
	github.com/google/renameio v1.0.1 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hhrutter/lzw v1.0.0 // indirect
	github.com/hhrutter/pkcs7 v0.2.2 // indirect
	github.com/hhrutter/tiff v1.0.3 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jackmordaunt/icns/v3 v3.0.1 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1 // indirect
	github.com/pjbgf/sha1cd v0.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/richardlehane/mscfb v1.0.7 // indirect
	github.com/richardlehane/msoleps v1.0.6 // indirect
	github.com/rs/dnscache v0.0.0-20230804202142-fc85eb664529 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/sergeymakinen/go-bmp v1.0.0 // indirect
	github.com/sergeymakinen/go-ico v1.0.0-beta.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tadvi/systray v0.0.0-20190226123456-11a2b8fa57af // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tiendc/go-deepcopy v1.7.2 // indirect
	github.com/viterin/partial v1.1.0 // indirect
	github.com/viterin/vek v0.4.2 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	github.com/ysmood/fetchup v0.2.3 // indirect
	github.com/ysmood/goob v0.4.0 // indirect
	github.com/ysmood/got v0.40.0 // indirect
	github.com/ysmood/gson v0.7.3 // indirect
	github.com/ysmood/leakless v0.9.0 // indirect
	github.com/yuin/goldmark v1.7.17 // indirect
	github.com/yuin/goldmark-emoji v1.0.6 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.3 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
