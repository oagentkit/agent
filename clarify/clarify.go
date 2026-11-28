// Package clarify 澄清/标准化词表内核：口语→规范维度词条（term_map）的模型、外置
// 加载、内在校验，以及澄清反问的回答消解（序数指代 → 选项词匹配（精确相等优先 +
// 否定前缀防护的包含）→ 逃生兜底）。
//
// 生命周期: Experimental——词表与回答消解纯逻辑未过一个 minor 无 breaking 窗口
// （api-compat.md 晋级条件一），初值如实记为不承诺兼容。
//
// 归一/澄清只补充语义不改路由：词表命中产出宿主槽位（词表治本，LLM 同名槽位不可
// 推翻）；挂起态存取、反问状态机与注入段渲染留宿主——本包只管「表长什么样、回答
// 怎么消解」这两件纯逻辑。
//
// 沉淀自 bianque 输入标准化层 + 消歧 clarify（v0.9.7）：序数解析在宿主已有消歧衔接
// 与标准化反问两处消费者。
package clarify

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Options 澄清反问模板：Question 提问、Options 可选答案（声明序=序数序）、
// Fallback 逃生选项（「不清楚，全面查」——选它=放弃细分走全量）。
type Options struct {
	Question string   `yaml:"question"`
	Options  []string `yaml:"options"`
	Fallback string   `yaml:"fallback"`
}

// Entry 词表条目：Word 由宿主词表内核子串匹配（如 router.KeywordHit，含否定守门），
// 声明序即匹配优先级——specific 词条须声明在泛词之前。
type Entry struct {
	Word      string   `yaml:"word"`
	Dimension string   `yaml:"dimension"` // 标准维度键 ^[a-z][a-z0-9_]*$
	Terms     []string `yaml:"terms"`     // 标准术语（宿主注入段展示用）
	Vague     bool     `yaml:"vague"`     // true=模糊维度：宿主场景命中时可反问补齐细分
	Clarify   *Options `yaml:"clarify"`   // vague=true 时必填；非 vague 不得配置
	Domain    string   `yaml:"domain"`    // 空=全域；否则宿主校验其为已注册域
}

// LoadVocab 读 <fsys>/<path> 外置词表（yaml 根键 term_map）。缺文件返回 (nil, nil)
// （零词表=宿主零行为变化）；存在但读取/解析失败即报错（拒绝半份词表静默上线）。
func LoadVocab(fsys fs.FS, path string) ([]Entry, error) {
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("clarify: 读取 %s: %w", path, err)
	}
	var doc struct {
		TermMap []Entry `yaml:"term_map"`
	}
	// KnownFields 严格解码（oagentkit/agent#43）：词表文件来自内容资产仓，
	// 键名手写漂移是常态——terms: 误写成 items:/term: 时宽松解码会静默忽略，
	// 加载「成功」但注入段缺术语（问答质量慢性下降，无任何信号指向词表）。
	// 严格模式下拼错键即报错，与「拒绝半份词表静默上线」的既有立场同向。
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("clarify: %s 解析: %w", path, err)
	}
	return doc.TermMap, nil
}

// dimRe 标准维度键规范：小写拉丁开头，仅小写字母/数字/下划线。
var dimRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate 词表内在校验：word 非空且唯一、维度键规范、vague⟺clarify 完整
// （question/options/fallback 必带，逃生选项必配）。域注册存在性与路由词冲突等
// 宿主拓扑校验不在此——宿主在装配期自校（缺信息，也无从给出准确告警文案）。
func Validate(entries []Entry) error {
	owner := map[string]string{}
	for i := range entries {
		e := &entries[i]
		if e.Word == "" {
			return fmt.Errorf("clarify: term_map 第 %d 条缺 word", i+1)
		}
		if prev, hit := owner[e.Word]; hit {
			return fmt.Errorf("clarify: term_map 词 %q 重复（%s / %s）", e.Word, prev, e.Dimension)
		}
		owner[e.Word] = e.Dimension
		if !dimRe.MatchString(e.Dimension) {
			return fmt.Errorf("clarify: term_map %q 维度键非法 %q（须 ^[a-z][a-z0-9_]*$）", e.Word, e.Dimension)
		}
		if e.Vague && (e.Clarify == nil || e.Clarify.Question == "" || len(e.Clarify.Options) == 0 || e.Clarify.Fallback == "") {
			return fmt.Errorf("clarify: term_map %q vague=true 须配 clarify{question,options,fallback}（逃生选项必带）", e.Word)
		}
		if !e.Vague && e.Clarify != nil {
			return fmt.Errorf("clarify: term_map %q 非 vague 不得配置 clarify", e.Word)
		}
	}
	return nil
}

// ValidateTermsStrict 词目术语集的结构性严格检查（opt-in，oagentkit/agent#43）：
// 非 vague 条目 Terms 为空即报错——「有维度无约束」的静默降级形态（注入段为空，
// 问答质量慢性下降且无信号）。不并入 Validate：词条只做路由、无术语展示是合法
// 形态（既有测试钉住的语义）；需要术语注入完备性的宿主在装配期显式调用本函数。
// 键名拼错（terms: → items:）由 LoadVocab 的 KnownFields 在解析层拦下，本函数
// 兜的是「合法键但真没写术语」的结构层。
func ValidateTermsStrict(entries []Entry) error {
	for i := range entries {
		e := &entries[i]
		if !e.Vague && len(e.Terms) == 0 {
			return fmt.Errorf("clarify: term_map 第 %d 条 %q 维度 %q 无 terms（非 vague 词目须配标准术语集，否则注入段为空）", i+1, e.Word, e.Dimension)
		}
	}
	return nil
}

// OrdinalIndex 解析回答里的序数指代（第一个/第1个/第 2 个/1./选项二/选一/第10个/
// 第十个/第二十三个），返回 1-based 序号。汉字与阿拉伯数字双形态；阿拉伯数字支持
// 多位数、汉字支持 一~九 与 十/二十/二十三 合成形态（agent#147：此前单 rune 限制
// 让「第10个」及以上序数整类解析失败，选项数 >9 时明确选择被当换话题）。
// 仅当剥离序数构式后剩余部分为空、整体就是序数指代时命中（「1个进程在跑」
// 「第10个进程在跑」这类普通句子不误伤）。前缀/后缀剥离都循环至不动点
// （oagentkit/agent#108）：中文回答常态带尾句读，「第一个。」「第1个、」是
// 「序数词缀 + 尾标点」双层后缀——单遍过表只剥一层，整类形态解析失败后被
// 当「换话题」丢弃。
func OrdinalIndex(lower string) (int, bool) {
	s := strings.TrimSpace(lower)
	if s == "" {
		return 0, false
	}
	s = strings.ReplaceAll(s, " ", "")
	s = trimPrefixLoop(s, []string{"选项", "选", "第"})
	s = trimSuffixLoop(s, []string{"个", "选项", "方案", "的", ".", "。", "、"})
	return parseOrdinalNumber(s)
}

// parseOrdinalNumber 把剥离序数构式后的整串解析为 1-based 序号：
//   - 阿拉伯数字：整串全为 ASCII 数字即成立（多位数，agent#147）；
//   - 汉字数字：单字 一~九；「十」（10）、「十X」（11-19）、「X十」（20-90）、
//     「X十Y」（21-99）按十位/个位合成。
//
// 非纯数字串（「1个进程在跑」剥缀后的残留句子）返回 false，不误伤普通回答。
func parseOrdinalNumber(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	allDigits := true
	for _, r := range s {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return 0, false
		}
		return n, true
	}
	cnDigit := func(r rune) (int, bool) {
		n, ok := cnDigits[r]
		return n, ok
	}
	if s == "十" { // 单独「十」= 10（cnDigits 只收个位，十位在下方合成）
		return 10, true
	}
	r := []rune(s)
	switch len(r) {
	case 1:
		return cnDigit(r[0])
	case 2:
		if r[0] == '十' { // 十X = 10+X
			if n, ok := cnDigit(r[1]); ok {
				return 10 + n, true
			}
			return 0, false
		}
		if r[1] == '十' { // X十 = X*10
			if n, ok := cnDigit(r[0]); ok {
				return n * 10, true
			}
		}
	case 3:
		if r[1] == '十' { // X十Y = X*10+Y
			tens, ok1 := cnDigit(r[0])
			ones, ok2 := cnDigit(r[2])
			if ok1 && ok2 {
				return tens*10 + ones, true
			}
		}
	}
	return 0, false
}

// cnDigits 汉字个位数字表（十位合成见 parseOrdinalNumber）。
var cnDigits = map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}

// trimPrefixLoop 逐前缀循环剥离至不动点（表任一前缀不再命中即止）。
func trimPrefixLoop(s string, prefixes []string) string {
	for changed := true; changed; {
		changed = false
		for _, p := range prefixes {
			if t := strings.TrimPrefix(s, p); t != s {
				s, changed = t, true
			}
		}
	}
	return s
}

// trimSuffixLoop 逐后缀循环剥离至不动点（表任一后缀不再命中即止）。
func trimSuffixLoop(s string, suffixes []string) string {
	for changed := true; changed; {
		changed = false
		for _, suf := range suffixes {
			if t := strings.TrimSuffix(s, suf); t != s {
				s, changed = t, true
			}
		}
	}
	return s
}

// negWords 否定前缀词表：答案以这些否定词**直接拼接**选项词时构成该选项的否定
// 形态（「不是」对「是」、「不同意删除」对「删除」、「没意见」对「意见」、
// 「not yes」对「yes」、「no thanks」对「thanks」）。
//
// 单字与常见**多字**形态都必须在表内（agent#129）：判定是逐词精确拼接
// （a == neg+opt），缺词时拼接不成立 ⇒ 否定检测落空 ⇒ 第二轮宽松 Contains
// 把**被否定的选项**命中返回——「不同意删除数据库」曾被消解成「删除数据库」，
// 用户明确拒绝的操作被选中执行。
//
// 英文覆盖（agent#146）：strip 只做 ToLower + 删空格，故英文否定按**无空格
// 拼接形态**登记（「not yes」→「notyes」= "not"+"yes"）；缩写同时收带撇号与
// 去撇号两种拼写（撇号不被 strip 删除）。前置否定才在范围内（「I don't think
// so」这类后置/从句否定仍属宽松 Contains 的既有边界）。
var negWords = []string{
	// 单字否定
	"不", "没", "别", "非", "无", "莫", "勿", "未",
	// 多字否定（agent#129 缺口补齐）：不是/不会/不能/不可以/不同意/不需要 这类
	// 多字词若不在表内，「多字否定词 + 选项词」的整串拼接对任何单字词都不成立。
	"不要", "不用", "没有", "不是", "不会", "不能", "不可以",
	"不同意", "不需要", "不支持", "不许", "不准", "不让", "未曾",
	// 英文否定（agent#146）：not yes / no thanks / never yes / don't yes …
	"not", "no", "never",
	"dont", "don't", "doesnt", "doesn't", "didnt", "didn't",
	"cannot", "cant", "can't", "wont", "won't",
	"wouldnt", "wouldn't", "shouldnt", "shouldn't",
	"isnt", "isn't", "arent", "aren't", "wasnt", "wasn't", "werent", "weren't",
	"aint", "ain't",
}

// isNegatedForm 报告 a（已 strip）是否为否定词直接拼接 opt（已 strip）的否定形态
// （a == neg+opt，对词表逐词独立判定，与词序/词长无关）。真实覆盖范围：
//   - 表内否定词（中英单字 + 多字形态）与选项词的**整串精确拼接**——「不是删除
//     数据库」对「删除数据库」（agent#129：多字词缺表时拼接落空，被 Contains
//     误命中）、「notyes」对「yes」（agent#146：纯中文词表让英文否定整体缺席）；
//   - 「我觉得不是删除数据库」这类**前置填充的松散否定不在范围**——那是宽松
//     Contains 容错的既有边界，不是本函数的缺口。
func isNegatedForm(a, opt string) bool {
	for _, neg := range negWords {
		if a == neg+opt {
			return true
		}
	}
	return false
}

// ResolveAnswer 澄清回答消解：序数指代（1-based 对 options 序，越界=放弃）→ 选项词
// 匹配（双侧去空格；先全量精确相等，再退宽松 Contains，且否定前缀形态不做 Contains
// 命中，见下）→ 逃生选项（答案是其前缀也算，如「不清楚」对「不清楚，全面查」）。
// picked=命中的选项词（fallback 命中时为空）；ok=false=回答不在本题语义内（宿主按
// 「用户换话题」处置，清挂起走正常分类）。
//
// 选项匹配两轮（oagentkit/agent#69）：第一轮全量精确相等；第二轮宽松 Contains 兜底
// （保留长选项的容错，「请删除数据库」对「删除数据库」仍命中）。此前单轮 Contains
// 会把「不是」误命中「是」——否定前缀被子串吞掉，用户明确拒绝被当成同意（P2）。
// 防护规则：答案=否定词直接拼接选项词时，该选项视为被否定而非被选择；在恰好两个
// 选项的强制选择题里，对一个选项的否定无歧义地指另一个（「不是」在 {是,否} 里就是
// 「否」、「不同意」在 {同意,拒绝} 里就是「拒绝」），选项数 ≠2 时否定无法定位到单一
// 选项，不做翻转（交由逃生/换话题路径处置）。
func ResolveAnswer(options []string, fallback, answer string) (picked string, isFallback bool, ok bool) {
	lower := strings.TrimSpace(strings.ToLower(answer))
	if lower == "" {
		return "", false, false
	}
	if n, ok := OrdinalIndex(lower); ok {
		if n >= 1 && n <= len(options) {
			return options[n-1], false, true
		}
		return "", false, false
	}
	strip := func(s string) string { return strings.ReplaceAll(strings.ToLower(s), " ", "") }
	a := strip(answer)
	if a == "" {
		return "", false, false
	}
	// 第一轮：strip 后精确相等优先——答案与后声明的更长选项完全一致时命中更长者
	// （「删除数据库」不再被先声明的「删除」截胡）。
	stripped := make([]string, len(options))
	for i, o := range options {
		stripped[i] = strip(o)
		if stripped[i] != "" && a == stripped[i] {
			return o, false, true
		}
	}
	// 第二轮：宽松 Contains；否定前缀形态（「不是」「不同意」「不可以」……）的
	// 选项被否决，不经子串命中。negIdx/negCount 记录被否决的选项供二元翻转。
	// 取**最长**命中而非声明序首个（oagentkit/agent#109）：泛词在前是本包
	// 推荐的声明序（specific 词条须声明在泛词之前），「请删除数据库」曾按
	// 声明序命中先声明的「删除」——精确轮修掉的截胡形态在带前缀的回答里
	// 复发。全部命中后取最长（否定判定照旧对最终候选做）。
	negIdx, negCount := -1, 0
	best := -1
	for i, so := range stripped {
		if so == "" || !strings.Contains(a, so) {
			continue
		}
		if isNegatedForm(a, so) {
			if negCount == 0 {
				negIdx = i
			}
			negCount++
			continue
		}
		if best < 0 || utf8.RuneCountInString(so) > utf8.RuneCountInString(stripped[best]) {
			best = i
		}
	}
	if best >= 0 {
		return options[best], false, true
	}
	// 二元否定翻转：恰有两个选项且恰好一个被否定形态否决时，答案指向另一个；
	// 多个选项同被否定（歧义）或选项数 ≠2 时不翻转。
	if negCount == 1 && len(options) == 2 {
		return options[1-negIdx], false, true
	}
	fb := strip(fallback)
	if fb != "" {
		// 逃生命中两种形态：答案包含完整逃生短语（明确选择）；答案是逃生短语的
		// 前缀（「不清楚」对「不清楚，全面查」的缩写形态）。此前是双向子串包含
		//——「全面」「不」这类词也命中，把有效回答误消解为放弃细分（第六轮审计）
		if strings.Contains(a, fb) ||
			(utf8.RuneCountInString(a) >= 2 && strings.HasPrefix(fb, a)) {
			return "", true, true
		}
	}
	return "", false, false
}
