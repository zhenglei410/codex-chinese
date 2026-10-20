// force-codex-chinese (Go) —— 零第三方依赖，单文件小体积跨平台。
// 机制：启动 Codex（带 --remote-debugging-port）→ CDP 注入 prelude →
//       在渲染进程主世界里给 Statsig 的 i18n 开关（"72216192"）打补丁。
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
    if (!obj || typeof obj !== "object") return;
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
  }
  function patchClient(client) {
    if (!client || typeof client !== "object") return;
    function wrap(name) {
      var orig = client[name] && client[name].bind(client);
      if (!orig) return;
      client[name] = function (n, o) {
        var r = orig(n, o);
        return n === "72216192" ? patchValue(r) : r;
      };
    }
    if (client.__zhClientPatched) return;
    wrap("getLayer");
    wrap("getDynamicConfig");
    if (typeof client._getLayerImpl === "function") {
      var o = client._getLayerImpl.bind(client);
      client._getLayerImpl = function (n) {
        var r = o.apply(null, arguments);
        return n === "72216192" ? patchValue(r) : r;
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
	fmt.Println("== 强制 Codex 中文界面 ==")

	if !*flagNoLaunch && !endpointReady(*flagPort) {
		exe := findCodex()
		if exe == "" {
			fmt.Println("找不到 Codex。请用 -codex 指定路径，或先退出 Codex 再运行。")
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
		fmt.Println("已触发刷新，中文界面将在加载后生效。")
	}
	fmt.Println("完成：Codex 已强制中文。")
}

// ---------------- 环境诊断（供 AI 部署 / CI 校验） ----------------
func runDoctor() {
	exe := findCodex()
	ready := endpointReady(*flagPort)
	fmt.Printf("version: %s\n", version)
	fmt.Printf("platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if exe == "" {
		fmt.Println("codex: not-found")
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
func findCodex() string {
	if *flagCodex != "" {
		if _, err := os.Stat(*flagCodex); err == nil {
			return *flagCodex
		}
		return ""
	}
	var cands []string
	switch runtime.GOOS {
	case "windows":
		la := os.Getenv("LOCALAPPDATA")
		cands = []string{
			filepath.Join(la, "Programs", "Codex", "Codex.exe"),
			filepath.Join(la, "Programs", "codex", "Codex.exe"),
			filepath.Join(la, "Microsoft", "WindowsApps", "Codex.exe"),
			`C:\Program Files\Codex\Codex.exe`,
			`C:\Program Files (x86)\Codex\Codex.exe`,
		}
	case "darwin":
		home, _ := os.UserHomeDir()
		cands = []string{
			"/Applications/Codex.app/Contents/MacOS/Codex",
			filepath.Join(home, "Applications", "Codex.app", "Contents", "MacOS", "Codex"),
		}
	default:
		cands = []string{"/usr/bin/codex", "/usr/local/bin/codex", "/opt/Codex/codex"}
	}
	for _, c := range cands {
		if c != "" {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	return ""
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
			ID    int             `json:"id"`
			Error *struct{ Message string } `json:"error"`
			Result json.RawMessage `json:"result"`
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
