package bff_proxy_client

import (
	"context"
	_ "embed"
	"encoding/xml"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

const (
	langpackCodeZhHans = "classic-zh-cn"
	langpackCodeEn     = "en"
	langpackVersion    = int32(1)
)

var (
	//go:embed langpack-resources/zh-CN/Localizable-iOS.strings
	langpackZhIOSRaw string
	//go:embed langpack-resources/source/Localizable-iOS.strings
	langpackEnIOSRaw string
	//go:embed langpack-resources/zh-CN/strings-android.xml
	langpackZhAndroidRaw string
	//go:embed langpack-resources/source/strings-android.xml
	langpackEnAndroidRaw string

	langpackOnce sync.Once
	langpackData *langpackCatalog
	langpackErr  error

	langpackAndroidZhOnce sync.Once
	langpackAndroidZh     map[string]string
	langpackAndroidZhErr  error

	langpackAndroidEnOnce sync.Once
	langpackAndroidEn     map[string]string
	langpackAndroidEnErr  error

	langpackIOSZhOnce sync.Once
	langpackIOSZh     map[string]string
	langpackIOSZhErr  error

	langpackIOSEnOnce sync.Once
	langpackIOSEn     map[string]string
	langpackIOSEnErr  error

	iosStringsPattern = regexp.MustCompile(`^\s*"((?:\\.|[^"])*)"\s*=\s*"((?:\\.|[^"])*)";\s*$`)
)

type langpackCatalog struct {
	language *mtproto.LangPackLanguage
}

type androidStringsFile struct {
	Strings []androidStringNode `xml:"string"`
}

type androidStringNode struct {
	Name  string `xml:"name,attr"`
	Value string `xml:",chardata"`
}

func getLangpackCatalog() (*langpackCatalog, error) {
	langpackOnce.Do(func() {
		language := mtproto.MakeTLLangPackLanguage(&mtproto.LangPackLanguage{
			Name:            "Simplified Chinese",
			NativeName:      "简体中文",
			LangCode:        langpackCodeZhHans,
			PluralCode:      "zh-hans",
			StringsCount:    int32(estimateAndroidStringsCount(langpackZhAndroidRaw)),
			TranslatedCount: int32(estimateAndroidStringsCount(langpackZhAndroidRaw)),
			TranslationsUrl: "https://translations.telegram.org/classic-zh-cn/",
			Official:        true,
		}).To_LangPackLanguage()

		langpackData = &langpackCatalog{
			language: language,
		}
	})

	return langpackData, langpackErr
}

func parseAppleStrings(raw string) (map[string]string, error) {
	result := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "*/") {
			continue
		}
		matches := iosStringsPattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		key, err := strconv.Unquote(`"` + matches[1] + `"`)
		if err != nil {
			continue
		}
		value, err := strconv.Unquote(`"` + matches[2] + `"`)
		if err != nil {
			continue
		}
		result[key] = value
	}
	return result, nil
}

func parseAndroidStrings(raw string) (map[string]string, error) {
	var resources androidStringsFile
	if err := xml.Unmarshal([]byte(raw), &resources); err != nil {
		return nil, err
	}

	result := make(map[string]string, len(resources.Strings))
	for _, item := range resources.Strings {
		if item.Name == "" {
			continue
		}
		// Android client only escapes a very narrow form of '&' when it writes
		// remote langpacks back to XML, so keep entity-safe ampersands here.
		result[item.Name] = strings.ReplaceAll(strings.TrimSpace(item.Value), "&", "&amp;")
	}
	return result, nil
}

func estimateAndroidStringsCount(raw string) int {
	return strings.Count(raw, "<string ")
}

func normalizeLangCode(code string) string {
	code = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(code, "_", "-")))
	switch code {
	case "", "zh", "zh-cn", "zh-hans", "zh-hans-cn", "classic-zh-cn", "classic-zh-hans-cn", "zhlangcn":
		return langpackCodeZhHans
	case "en", "en-us", "en-gb":
		return langpackCodeEn
	default:
		return code
	}
}

func loadLangpackStringMap(raw string, parser func(string) (map[string]string, error), once *sync.Once, cached *map[string]string, cachedErr *error) (map[string]string, error) {
	once.Do(func() {
		*cached, *cachedErr = parser(raw)
	})
	return *cached, *cachedErr
}

func langpackStringsForClient(ctx context.Context, rpcMetaData *metadata.RpcMetadata, catalog *langpackCatalog, requestedLangPack string, langCode string) (map[string]string, error) {
	client := ""
	langPack := strings.ToLower(strings.TrimSpace(requestedLangPack))

	md := rpcMetaData
	if md == nil {
		md = metadata.RpcMetadataFromIncoming(ctx)
	}
	if md != nil {
		client = strings.ToLower(md.GetClient())
		if langPack == "" {
			langPack = strings.ToLower(md.GetLangpack())
		}
	}

	isAndroid := client == "android" || langPack == "android"

	switch normalizeLangCode(langCode) {
	case langpackCodeZhHans:
		if isAndroid {
			return loadLangpackStringMap(langpackZhAndroidRaw, parseAndroidStrings, &langpackAndroidZhOnce, &langpackAndroidZh, &langpackAndroidZhErr)
		}
		return loadLangpackStringMap(langpackZhIOSRaw, parseAppleStrings, &langpackIOSZhOnce, &langpackIOSZh, &langpackIOSZhErr)
	case langpackCodeEn:
		if isAndroid {
			return loadLangpackStringMap(langpackEnAndroidRaw, parseAndroidStrings, &langpackAndroidEnOnce, &langpackAndroidEn, &langpackAndroidEnErr)
		}
		return loadLangpackStringMap(langpackEnIOSRaw, parseAppleStrings, &langpackIOSEnOnce, &langpackIOSEn, &langpackIOSEnErr)
	default:
		return nil, nil
	}
}

func buildLangPackStringVector(requestedKeys []string, values map[string]string) *mtproto.Vector_LangPackString {
	vector := &mtproto.Vector_LangPackString{Datas: []*mtproto.LangPackString{}}
	if values == nil {
		return vector
	}

	appendKey := func(key string) {
		if value, ok := values[key]; ok {
			vector.Datas = append(vector.Datas, mtproto.MakeTLLangPackString(&mtproto.LangPackString{
				Key:   key,
				Value: value,
			}).To_LangPackString())
		}
	}

	if len(requestedKeys) > 0 {
		for _, key := range requestedKeys {
			appendKey(key)
		}
		return vector
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendKey(key)
	}

	return vector
}

func buildLangPackDifference(langCode string, fromVersion int32, values map[string]string) mtproto.TLObject {
	version := fromVersion
	stringsVector := []*mtproto.LangPackString{}
	if fromVersion < langpackVersion {
		version = langpackVersion
		stringsVector = buildLangPackStringVector(nil, values).Datas
	}

	return mtproto.MakeTLLangPackDifference(&mtproto.LangPackDifference{
		LangCode:    normalizeLangCode(langCode),
		FromVersion: fromVersion,
		Version:     version,
		Strings:     stringsVector,
	}).To_LangPackDifference()
}

