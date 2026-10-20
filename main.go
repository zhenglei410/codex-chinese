// force-codex-chinese (Go) —— 零第三方依赖，单文件小体积跨平台。
// 机制：启动 Codex（带 --remote-debugging-port）→ CDP 注入 prelude →
//
//	在渲染进程主世界里给 Statsig 的 i18n 开关（"72216192"）打补丁。
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const prelude = `(function () {
  var locale = "zh-CN";
  var key = "codexplus:zhcn:1";
  if (window.__codexPlusForceZhInstalled === key) return;
  window.__codexPlusForceZhInstalled = key;
  var languages = [locale, "zh", "en-US", "en"];
  function defineNav(name, value) {
    try { Object.defineProperty(Navigator.prototype, name, { configurable: true, get: function () { return value; } }); }
    catch (e) { try { Object.defineProperty(navigator, name, { configurable: true, get: function () { return value; } }); } catch (e2) {} }
  }
  defineNav("language", locale);
  defineNav("languages", languages);
  function patchValue(obj) {
    if (!obj || typeof obj !== "object") return obj;
    var base = (obj.__value && typeof obj.__value === "object") ? obj.__value : ((obj.value && typeof obj.value === "object") ? obj.value : {});
    var next = Object.assign({}, base, { enable_i18n: true, locale_source: "SYSTEM" });
    try { obj.__value = next; } catch (e) {}
    try { obj.value = next; } catch (e) {}
    if (typeof obj.get === "function" && !obj.__zhPatched) {
      var orig = obj.get.bind(obj);
      obj.get = function (k, f) {
        if (k === "enable_i18n") return true;
        if (k === "locale_source") return "SYSTEM";
        return orig(k, f);
      };
      obj.__zhPatched = true;
    }
    return obj;
  }
  function patchClient(client) {
    if (!client || typeof client !== "object") return;
    function wrap(name) {
      var orig = client[name] && client[name].bind(client);
      if (!orig) return;
      client[name] = function (n, o) {
        var r = orig(n, o);
        // 必须原样返回 r。写成 return patchValue(r) 会在 patchValue 无返回值时
        // 把 Layer 变成 undefined，导致 Codex 拿不到 i18n 开关（汉化静默失效）。
        if (n === "72216192") patchValue(r);
        return r;
      };
    }
    if (client.__zhClientPatched) return;
    wrap("getLayer");
    wrap("getDynamicConfig");
    if (typeof client._getLayerImpl === "function") {
      var o = client._getLayerImpl.bind(client);
      client._getLayerImpl = function (n) {
        var r = o.apply(null, arguments);
        if (n === "72216192") patchValue(r);
        return r;
      };
    }
    client.__zhClientPatched = true;
    try { patchValue(client.getDynamicConfig("72216192", { disableExposureLog: true })); } catch (e) {}
    try { if (client.getLayer) patchValue(client.getLayer("72216192", { disableExposureLog: true })); } catch (e) {}
  }
  function clients() {
    var root = window.__STATSIG__ || globalThis.__STATSIG__;
    if (!root || typeof root !== "object") return [];
    var list = [root.firstInstance, typeof root.instance === "function" ? root.instance() : null];
    if (root.instances && typeof root.instances === "object") {
      Object.keys(root.instances).forEach(function (k) { list.push(root.instances[k]); });
    }
    return list.filter(function (c, i, a) { return c && typeof c === "object" && a.indexOf(c) === i; });
  }
  function patchRoot(root) {
    if (!root || typeof root !== "object" || root.__zhRootPatched) return;
    root.__zhRootPatched = true;
    ["firstInstance", "instance"].forEach(function (k) {
      var cur;
      try { cur = root[k]; } catch (e) { return; }
      var c = (typeof cur === "function" && k === "instance") ? cur.call(root) : cur;
      patchClient(c);
      try {
        Object.defineProperty(root, k, {
          configurable: true,
          get: function () { return cur; },
          set: function (n) { cur = (typeof n === "function" && k === "instance") ? n.call(root) : n; patchClient(cur); }
        });
      } catch (e) {}
    });
  }
  function installRootSetter() {
    var d = Object.getOwnPropertyDescriptor(window, "__STATSIG__");
    if (d && d.configurable === false) return;
    var cur = window.__STATSIG__;
    patchRoot(cur);
    try {
      Object.defineProperty(window, "__STATSIG__", {
        configurable: true,
        get: function () { return cur; },
        set: function (n) { cur = n; patchRoot(n); clients().forEach(patchClient); }
      });
    } catch (e) {}
  }
  installRootSetter();
  patchRoot(window.__STATSIG__ || globalThis.__STATSIG__);
  clients().forEach(patchClient);
  var start = Date.now();
  var timer = setInterval(function () {
    installRootSetter();
    patchRoot(window.__STATSIG__ || globalThis.__STATSIG__);
    clients().forEach(patchClient);
    if (Date.now() - start > 5000) clearInterval(timer);
  }, 50);
  var reloadKey = "codexplus.zhcn.reload";
  var api = (typeof codexStateCall === "function") ? codexStateCall
    : ((window.__CODEX_STATE__ && typeof window.__CODEX_STATE__.setSetting === "function")
      ? function (m, p) { return window.__CODEX_STATE__.setSetting(p.key, p.value); }
      : null);
  if (api) {
    var done = function (v) {
      try {
        if (window.sessionStorage.getItem(reloadKey) === JSON.stringify(v)) return;
        window.sessionStorage.setItem(reloadKey, JSON.stringify(v));
      } catch (e) {}
      window.location.reload();
    };
    Promise.resolve(api("get-setting", { key: "localeOverride" })).then(function (r) {
      if (r && r.value === locale) { try { window.sessionStorage.removeItem(reloadKey); } catch (e) {} return; }
      Promise.resolve(api("set-setting", { key: "localeOverride", value: locale }))
        .then(function () { done(locale); }).catch(function () {});
    }).catch(function () {});
  }
})();`

// version 由构建时注入：-ldflags "-X main.version=v1.0.0"
var version = "dev"

var (
	flagCodex    = flag.String("codex", "", "Codex 可执行文件路径")
	flagPort     = flag.Int("port", 9229, "CDP 调试端口")
	flagWait     = flag.Int("wait", 20, "等待端口就绪的秒数")
	flagNoReload = flag.Bool("no-reload", false, "只注册脚本，不刷新")
	flagNoLaunch = flag.Bool("no-launch", false, "只注入已开调试端口的实例")
	flagVersion  = flag.Bool("version", false, "打印版本并退出")
	flagDoctor   = flag.Bool("doctor", false, "诊断环境并退出（供 AI/CI 校验）")
	flagVerify   = flag.Bool("verify", false, "校验当前 Codex 是否真的已注入中文（退出码 0=通过）")
)

func main() {
	flag.Parse()
	if *flagVersion {
		fmt.Printf("codex-chinese %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
		return
	}
	if *flagDoctor {
		runDoctor()
		return
	}
	if *flagVerify {
		os.Exit(runVerify(*flagPort))
	}
	fmt.Println("== 强制 Codex 中文界面 ==")

	if !*flagNoLaunch && !endpointReady(*flagPort) {
		exe := findCodex()
		if exe == "" {
			explainCodexMissing()
			os.Exit(1)
		}
		fmt.Println("启动 Codex:", exe)
		cmd := exec.Command(exe, fmt.Sprintf("--remote-debugging-port=%d", *flagPort))
		cmd.Stdout, cmd.Stderr = nil, nil
		applyDetach(cmd)
		if err := cmd.Start(); err != nil {
			fmt.Println("启动失败:", err)
			os.Exit(1)
		}
		_ = cmd.Process.Release()
	}

	deadline := time.Now().Add(time.Duration(*flagWait) * time.Second)
	for !endpointReady(*flagPort) {
		if time.Now().After(deadline) {
			fmt.Printf("等待调试端口 %d 超时。\n", *flagPort)
			fmt.Println("如果 Codex 已在运行（含托盘），请先完全退出再试。")
			os.Exit(1)
		}
		time.Sleep(300 * time.Millisecond)
	}

	target, err := pickTarget(*flagPort)
	if err != nil {
		fmt.Println("获取页面目标失败:", err)
		os.Exit(1)
	}
	fmt.Println("注入目标:", target.URL)

	ws, err := dialWS(target.WSURL)
	if err != nil {
		fmt.Println("连接调试通道失败:", err)
		os.Exit(1)
	}
	defer ws.Close()

	if _, err := ws.call(1, "Page.enable", map[string]any{}); err != nil {
		fmt.Println("Page.enable 失败:", err)
		os.Exit(1)
	}
	if _, err := ws.call(2, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": prelude}); err != nil {
		fmt.Println("注入失败:", err)
		os.Exit(1)
	}
	fmt.Println("已注册注入脚本（对后续所有页面生效）。")

	if !*flagNoReload {
		if _, err := ws.call(3, "Page.reload", map[string]any{}); err != nil {
			fmt.Println("刷新失败:", err)
			os.Exit(1)
		}
		// 关键：注册的脚本与 CDP 会话同生命周期。必须等新文档真正执行完 prelude
		// 再断开，否则脚本会在加载前被撤销，注入等于没做。
		if waitInjected(ws, 15*time.Second) {
			fmt.Println("注入已生效（页面已加载）。")
		} else {
			fmt.Println("警告：页面中未检测到注入标记，请运行 -verify 复查。")
		}
	}
	fmt.Println("完成：Codex 已强制中文。")
}

// 轮询页面，确认 prelude 已经真正执行（注入标记出现）。
func waitInjected(ws *wsConn, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		res, err := ws.call(800, "Runtime.evaluate", map[string]any{
			"expression":    "!!window.__codexPlusForceZhInstalled",
			"returnByValue": true,
		})
		if err == nil {
			var w struct {
				Result struct {
					Value bool `json:"value"`
				} `json:"result"`
			}
			if json.Unmarshal(res, &w) == nil && w.Result.Value {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ---------------- 注入结果校验（供 AI 部署 / 用户自检） ----------------
// 在页面里读取注入标记、navigator 语言与 Statsig 的 enable_i18n。
const verifyExpr = `(function(){try{var r=window.__STATSIG__,i18n=null;try{var c=r&&(r.firstInstance||(typeof r.instance==="function"?r.instance():null));if(c&&c.getLayer){var l=c.getLayer("72216192",{disableExposureLog:true});i18n=l?String(l.get("enable_i18n",false)):"layer-undefined";}}catch(e){i18n="err";}return JSON.stringify({installed:!!window.__codexPlusForceZhInstalled,lang:String(navigator.language||""),i18n:i18n});}catch(e){return JSON.stringify({installed:false,lang:"",i18n:"err"});}})()`

// 退出码：0=已注入且语言为中文；1=未通过；2=无法连接（未开调试端口/Codex 未运行）
func runVerify(port int) int {
	if !endpointReady(port) {
		fmt.Printf("verify: cdp not-ready (127.0.0.1:%d)\n", port)
		fmt.Println("FAIL: Codex 未以调试端口启动，或已完全退出")
		return 2
	}
	t, err := pickTarget(port)
	if err != nil {
		fmt.Println("verify: target error:", err)
		return 2
	}
	ws, err := dialWS(t.WSURL)
	if err != nil {
		fmt.Println("verify: connect error:", err)
		return 2
	}
	defer ws.Close()

	res, err := ws.call(900, "Runtime.evaluate", map[string]any{
		"expression":    verifyExpr,
		"returnByValue": true,
	})
	if err != nil {
		fmt.Println("verify: evaluate error:", err)
		return 2
	}
	var wrap struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res, &wrap); err != nil {
		fmt.Println("verify: parse error:", err)
		return 2
	}
	var v struct {
		Installed bool   `json:"installed"`
		Lang      string `json:"lang"`
		I18n      string `json:"i18n"`
	}
	if err := json.Unmarshal([]byte(wrap.Result.Value), &v); err != nil {
		fmt.Println("verify: parse payload error:", err)
		return 2
	}
	fmt.Printf("verify: installed=%t lang=%s i18n=%s\n", v.Installed, v.Lang, v.I18n)
	ok := v.Installed && strings.HasPrefix(strings.ToLower(v.Lang), "zh")
	if ok && v.I18n == "false" {
		// 标记在、语言对，但图层里的开关仍是 false：说明补丁没作用到 Layer 上，
		// Codex 依然会按英文渲染。这是"注入了但没汉化"的典型形态。
		fmt.Println("FAIL: enable_i18n 仍为 false，补丁未覆盖 Statsig 图层")
		return 1
	}
	if ok {
		fmt.Println("PASS")
		return 0
	}
	fmt.Println("FAIL: 未检测到注入标记或语言不是中文")
	return 1
}

// ---------------- 环境诊断（供 AI 部署 / CI 校验） ----------------
func runDoctor() {
	exe := findCodex()
	ready := endpointReady(*flagPort)
	fmt.Printf("version: %s\n", version)
	fmt.Printf("platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if exe == "" {
		fmt.Println("codex: not-found")
		if *flagCodex != "" {
			fmt.Printf("codex_arg: %s (不可访问)\n", *flagCodex)
		}
		fmt.Println("candidates:")
		for i, c := range codexCandidates() {
			if i >= 12 {
				fmt.Println("  ...")
				break
			}
			fmt.Printf("  - %s\n", c)
		}
	} else {
		fmt.Printf("codex: %s\n", exe)
	}
	if ready {
		fmt.Printf("cdp: ready (127.0.0.1:%d)\n", *flagPort)
	} else {
		fmt.Printf("cdp: not-ready (127.0.0.1:%d)\n", *flagPort)
	}
	if exe == "" && !ready {
		os.Exit(2)
	}
}

// ---------------- 找 Codex ----------------
// resolveExe 接受 exe 路径或安装目录，返回可执行的 exe 路径。
// 允许传目录，是因为用户常常只知道安装目录。
func resolveExe(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"'`)
	if p == "" {
		return ""
	}
	if fi, err := os.Stat(p); err == nil {
		if fi.IsDir() {
			for _, name := range []string{"ChatGPT.exe", "Codex.exe", "codex.exe"} {
				c := filepath.Join(p, name)
				if _, e := os.Stat(c); e == nil {
					return c
				}
			}
			return ""
		}
		return p
	}
	return ""
}

// codexCandidates 返回按优先级排序的候选路径。
//
// 商店（MSIX）版 Codex 的可执行文件叫 ChatGPT.exe，安装在
//
//	C:\Program Files\WindowsApps\OpenAI.Codex_<版本>_<架构>__<哈希>\app\
//
// 版本号与哈希都是随机的，所以不能写死路径，只能先找应用执行别名、
// 再按 glob 匹配。
func codexCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		la := os.Getenv("LOCALAPPDATA")
		pf := os.Getenv("ProgramFiles")
		pf86 := os.Getenv("ProgramFiles(x86)")
		var out []string
		if la != "" {
			wa := filepath.Join(la, "Microsoft", "WindowsApps")
			out = append(out,
				// 应用执行别名：普通权限即可运行，商店版首选
				filepath.Join(wa, "ChatGPT.exe"),
				filepath.Join(wa, "Codex.exe"),
				filepath.Join(la, "Programs", "Codex", "Codex.exe"),
				filepath.Join(la, "Programs", "codex", "Codex.exe"),
				filepath.Join(la, "Programs", "ChatGPT", "ChatGPT.exe"),
			)
		}
		if pf != "" {
			out = append(out,
				filepath.Join(pf, "Codex", "Codex.exe"),
				filepath.Join(pf, "ChatGPT", "ChatGPT.exe"),
			)
		}
		if pf86 != "" {
			out = append(out, filepath.Join(pf86, "Codex", "Codex.exe"))
		}
		out = append(out, storeCandidates()...)
		return out
	case "darwin":
		home, _ := os.UserHomeDir()
		return []string{
			"/Applications/Codex.app/Contents/MacOS/Codex",
			filepath.Join(home, "Applications", "Codex.app", "Contents", "MacOS", "Codex"),
			"/Applications/ChatGPT.app/Contents/MacOS/ChatGPT",
		}
	default:
		return []string{"/usr/bin/codex", "/usr/local/bin/codex", "/opt/Codex/codex"}
	}
}

// storeCandidates 探测 MSIX / 商店版安装目录。
//
// 优先查注册表：C:\Program Files\WindowsApps 对普通用户**不可枚举**，
// glob 在那里永远返回空；而注册表里的 PackageRootFolder 可读，
// 拿到根目录后拼 app\ChatGPT.exe 就能用。
func storeCandidates() []string {
	out := storeCandidatesFromRegistry()

	// glob 只在管理员权限（能列 WindowsApps）时有效，作为补充
	patterns := []string{
		`C:\Program Files\WindowsApps\OpenAI.Codex_*\app\ChatGPT.exe`,
		`C:\Program Files\WindowsApps\OpenAI.Codex_*\app\Codex.exe`,
	}
	var globbed []string
	for _, p := range patterns {
		if m, err := filepath.Glob(p); err == nil {
			globbed = append(globbed, m...)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(globbed)))
	if len(globbed) > 6 {
		globbed = globbed[:6]
	}
	return append(out, globbed...)
}

func findCodex() string {
	if *flagCodex != "" {
		return resolveExe(*flagCodex)
	}
	// 环境变量兜底，方便批量部署时统一指定
	if v := os.Getenv("CODEX_CHINESE_CODEX"); v != "" {
		if p := resolveExe(v); p != "" {
			return p
		}
	}
	for _, c := range codexCandidates() {
		if p := resolveExe(c); p != "" {
			return p
		}
	}
	return ""
}

// explainCodexMissing 在找不到 Codex 时给出可执行的原因与出路，
// 而不是一句"找不到"让用户猜。
func explainCodexMissing() {
	if *flagCodex != "" {
		p := strings.Trim(strings.TrimSpace(*flagCodex), `"'`)
		if _, err := os.Stat(p); err != nil {
			fmt.Printf("指定的路径不可访问：%s\n", p)
			fmt.Printf("  原因：%v\n", err)
			if strings.Contains(strings.ToLower(p), `\windowsapps\`) {
				fmt.Println("  提示：WindowsApps 受系统保护，普通权限可能读不到或无法直接执行。")
				fmt.Println(`  请改用应用执行别名：-codex "%LOCALAPPDATA%\Microsoft\WindowsApps\ChatGPT.exe"`)
			}
			return
		}
		fmt.Printf("指定的路径是目录，但里面没有 ChatGPT.exe / Codex.exe：%s\n", p)
		return
	}
	fmt.Println("没有自动找到 Codex。可用 -codex 指定，常见写法：")
	fmt.Println(`  -codex "%LOCALAPPDATA%\Microsoft\WindowsApps\ChatGPT.exe"`)
	fmt.Println(`  -codex "C:\Program Files\WindowsApps\OpenAI.Codex_<版本>_x64__<哈希>\app\ChatGPT.exe"`)
	fmt.Println("也可以设置环境变量 CODEX_CHINESE_CODEX 指向同一个路径。")
}

// ---------------- 极简 HTTP（仅 localhost，无 TLS） ----------------
func httpGet(port int, path string) ([]byte, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req := "GET " + path + " HTTP/1.1\r\nHost: 127.0.0.1:" + strconv.Itoa(port) + "\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)

	// 状态行
	status, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.Contains(status, " 200 ") {
		return nil, fmt.Errorf("HTTP 状态异常: %s", strings.TrimSpace(status))
	}

	// 响应头（只读头，不依赖连接关闭，避免 keep-alive 阻塞）
	contentLength := int64(-1)
	chunked := false
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "content-length:") {
			contentLength, _ = strconv.ParseInt(strings.TrimSpace(line[len("content-length:"):]), 10, 64)
		}
		if strings.HasPrefix(lower, "transfer-encoding:") && strings.Contains(lower, "chunked") {
			chunked = true
		}
	}

	// 响应体：按 Content-Length / chunked 精确读取
	var body []byte
	switch {
	case chunked:
		for {
			sizeLine, err := br.ReadString('\n')
			if err != nil {
				return nil, err
			}
			sizeLine = strings.TrimSpace(sizeLine)
			if sizeLine == "" {
				continue
			}
			n, perr := strconv.ParseInt(sizeLine, 16, 64)
			if perr != nil || n == 0 {
				return body, nil
			}
			buf := make([]byte, n)
			if _, err := io.ReadFull(br, buf); err != nil {
				return nil, err
			}
			body = append(body, buf...)
			_, _ = br.ReadString('\n') // chunk 尾部 CRLF
		}
	case contentLength >= 0:
		body = make([]byte, contentLength)
		if _, err := io.ReadFull(br, body); err != nil {
			return nil, err
		}
	default:
		body, _ = io.ReadAll(br)
	}
	return body, nil
}

func endpointReady(port int) bool {
	_, err := httpGet(port, "/json/version")
	return err == nil
}

// ---------------- 目标选择 ----------------
type target struct {
	Type, Title, URL, WSURL string
}

func pickTarget(port int) (target, error) {
	body, err := httpGet(port, "/json")
	if err != nil {
		return target{}, err
	}
	var raw []struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		URL         string `json:"url"`
		WebSocketID string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return target{}, err
	}
	var pages []target
	for _, t := range raw {
		if t.Type == "page" && !strings.Contains(t.URL, "devtools") && t.WebSocketID != "" {
			pages = append(pages, target{t.Type, t.Title, t.URL, t.WebSocketID})
		}
	}
	if len(pages) == 0 {
		return target{}, errors.New("没有可注入的页面")
	}
	for _, p := range pages {
		u := strings.ToLower(p.URL)
		if strings.Contains(u, "app://") || strings.Contains(u, "index.html") || strings.Contains(u, "codex") {
			return p, nil
		}
	}
	return pages[0], nil
}

// ---------------- 手写 WebSocket 客户端 ----------------
type wsConn struct {
	c  net.Conn
	br *bufio.Reader
	id int
}

func dialWS(rawurl string) (*wsConn, error) {
	// 仅支持 ws://host:port/path
	rest := strings.TrimPrefix(rawurl, "ws://")
	slash := strings.IndexByte(rest, '/')
	host, path := rest, "/"
	if slash >= 0 {
		host, path = rest[:slash], rest[slash:]
	}
	c, err := net.DialTimeout("tcp", host, 5*time.Second)
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	var nonce [16]byte
	_, _ = rand.Read(nonce[:])
	key := base64.StdEncoding.EncodeToString(nonce[:])
	req := "GET " + path + " HTTP/1.1\r\nHost: " + host +
		"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key +
		"\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := c.Write([]byte(req)); err != nil {
		c.Close()
		return nil, err
	}
	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	if err != nil {
		c.Close()
		return nil, err
	}
	if !strings.Contains(status, "101") {
		c.Close()
		return nil, fmt.Errorf("WebSocket 握手失败: %s", strings.TrimSpace(status))
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			c.Close()
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	return &wsConn{c: c, br: br}, nil
}

func (w *wsConn) Close() { _ = w.c.Close() }

func (w *wsConn) writeFrame(op byte, payload []byte) error {
	var hdr []byte
	hdr = append(hdr, 0x80|op) // FIN + opcode
	n := len(payload)
	switch {
	case n < 126:
		hdr = append(hdr, 0x80|byte(n))
	case n < 65536:
		hdr = append(hdr, 0x80|126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		hdr = append(hdr, ext[:]...)
	}
	var mask [4]byte
	_, _ = rand.Read(mask[:])
	hdr = append(hdr, mask[:]...)
	masked := make([]byte, n)
	for i := 0; i < n; i++ {
		masked[i] = payload[i] ^ mask[i%4]
	}
	if _, err := w.c.Write(hdr); err != nil {
		return err
	}
	_, err := w.c.Write(masked)
	return err
}

func (w *wsConn) readMessage() ([]byte, error) {
	var msg []byte
	for {
		b0, err := w.br.ReadByte()
		if err != nil {
			return nil, err
		}
		b1, err := w.br.ReadByte()
		if err != nil {
			return nil, err
		}
		fin := b0&0x80 != 0
		op := b0 & 0x0f
		masked := b1&0x80 != 0
		n := int64(b1 & 0x7f)
		if n == 126 {
			var ext [2]byte
			if _, err := io.ReadFull(w.br, ext[:]); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint16(ext[:]))
		} else if n == 127 {
			var ext [8]byte
			if _, err := io.ReadFull(w.br, ext[:]); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint64(ext[:]))
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(w.br, mask[:]); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(w.br, payload); err != nil {
			return nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		switch op {
		case 0x8:
			return nil, io.EOF
		case 0x9:
			_ = w.writeFrame(0xA, payload) // pong
			continue
		case 0x1, 0x0:
			msg = append(msg, payload...)
			if fin {
				return msg, nil
			}
		}
	}
}

func (w *wsConn) call(id int, method string, params any) (json.RawMessage, error) {
	req := map[string]any{"id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, _ := json.Marshal(req)
	if err := w.writeFrame(0x1, b); err != nil {
		return nil, err
	}
	for {
		msg, err := w.readMessage()
		if err != nil {
			return nil, err
		}
		var resp struct {
			ID     int                       `json:"id"`
			Error  *struct{ Message string } `json:"error"`
			Result json.RawMessage           `json:"result"`
		}
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}
		if resp.ID == id {
			if resp.Error != nil {
				return nil, errors.New(resp.Error.Message)
			}
			return resp.Result, nil
		}
	}
}
