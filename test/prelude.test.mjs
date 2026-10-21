// 注入脚本（prelude）的回归测试。
//
// 为什么需要它：prelude 是内联在 main.go 里的 JS 字符串，Go 侧编译期完全看不到
// 它的逻辑错误。线上真实发生过两个只有端到端才会暴露的 bug：
//
//   B1. Page.addScriptToEvaluateOnNewDocument 注册的脚本与 CDP 会话同生命周期，
//       注入后立刻断开连接 → 脚本被撤销 → 什么都没注入。（Go 侧 waitInjected 守护）
//   B2. patchValue() 没有返回值，而包装器写的是 `return patchValue(r)`，
//       于是 getLayer("72216192") 返回 undefined，Codex 拿不到 i18n 开关，
//       表现为"注入标记在、但界面仍是英文"。（本文件守护）
//
// 运行：node test/prelude.test.mjs
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import vm from "node:vm";

const here = dirname(fileURLToPath(import.meta.url));
const mainGo = readFileSync(join(here, "..", "main.go"), "utf8");

const m = mainGo.match(/const prelude = `([\s\S]*?)`\n/);
if (!m) {
  console.error("FATAL: 无法从 main.go 中提取 prelude 常量");
  process.exit(1);
}
const prelude = m[1];

// ---- 构造一个最小可用的"浏览器"环境 ----
function makeLayer(initial) {
  const value = Object.assign({}, initial);
  return {
    __value: value,
    value: value,
    get(k, d) {
      return Object.prototype.hasOwnProperty.call(this.__value, k) ? this.__value[k] : d;
    },
  };
}

function makeClient() {
  const zh = makeLayer({ enable_i18n: false, locale_source: "SYSTEM" });
  const other = makeLayer({ enable_i18n: false, locale_source: "SYSTEM" });
  return {
    _zh: zh,
    _other: other,
    getLayer(name) {
      return name === "72216192" ? zh : other;
    },
    getDynamicConfig(name) {
      return name === "72216192" ? zh : other;
    },
  };
}

function makeEnv() {
  function Navigator() {}
  const win = {};
  // 真实浏览器里 navigator 是 Navigator 的实例，prelude 把 getter 装在
  // Navigator.prototype 上，所以测试环境必须用实例，否则原型 getter 不生效。
  const nav = new Navigator();
  const sandbox = {
    window: win,
    Navigator,
    navigator: nav,
    Object,
    JSON,
    String,
    Boolean,
    Promise,
    Date,
    Array,
    console,
    // The locale-setting retry loop is intentionally long-lived in a real
    // renderer. Unit tests have no Codex settings API, so do not keep Node alive
    // for the full retry window.
    setTimeout: () => 0,
    clearTimeout,
    // 关掉 50ms 轮询：测试里靠 __STATSIG__ 的 setter 立即生效，
    // 留着轮询只会让进程不退出。
    setInterval: () => 0,
    clearInterval: () => {},
  };
  sandbox.globalThis = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(prelude, sandbox, { filename: "prelude.js" });
  return { sandbox, win };
}

// ---- 断言框架 ----
let pass = 0;
const fails = [];
function check(name, fn) {
  try {
    fn();
    pass++;
    console.log(`  ok   ${name}`);
  } catch (e) {
    fails.push(`${name}: ${e.message}`);
    console.log(`  FAIL ${name}: ${e.message}`);
  }
}
function eq(actual, expected, label) {
  if (actual !== expected) {
    throw new Error(`${label} 期望 ${JSON.stringify(expected)}，实际 ${JSON.stringify(actual)}`);
  }
}

console.log("prelude 回归测试");

check("注入标记被写入", () => {
  const { win } = makeEnv();
  eq(win.__codexPlusForceZhInstalled, "codexplus:zhcn:1", "标记");
});

check("navigator.language 被改为 zh-CN", () => {
  const { sandbox } = makeEnv();
  eq(sandbox.navigator.language, "zh-CN", "navigator.language");
});

check("navigator.languages 首位是 zh-CN", () => {
  const { sandbox } = makeEnv();
  eq(sandbox.navigator.languages[0], "zh-CN", "languages[0]");
});

check("B2 回归：getLayer('72216192') 必须返回 Layer 对象而不是 undefined", () => {
  const { win } = makeEnv();
  const client = makeClient();
  win.__STATSIG__ = { firstInstance: client }; // 触发 setter → patchRoot → patchClient
  const layer = client.getLayer("72216192");
  if (layer === undefined || layer === null) {
    throw new Error("返回了 " + String(layer) + "（patchValue 无返回值会吃掉 Layer）");
  }
  eq(typeof layer.get, "function", "layer.get");
});

check("enable_i18n 被翻转为 true", () => {
  const { win } = makeEnv();
  const client = makeClient();
  win.__STATSIG__ = { firstInstance: client };
  eq(client.getLayer("72216192").get("enable_i18n", false), true, "enable_i18n");
});

check("locale_source 保持 SYSTEM", () => {
  const { win } = makeEnv();
  const client = makeClient();
  win.__STATSIG__ = { firstInstance: client };
  eq(client.getLayer("72216192").get("locale_source", ""), "SYSTEM", "locale_source");
});

check("底层 __value / value 都被改写（不只改 get）", () => {
  const { win } = makeEnv();
  const client = makeClient();
  win.__STATSIG__ = { firstInstance: client };
  const layer = client.getLayer("72216192");
  eq(layer.__value.enable_i18n, true, "__value.enable_i18n");
  eq(layer.value.enable_i18n, true, "value.enable_i18n");
});

check("不误伤其它 Layer", () => {
  const { win } = makeEnv();
  const client = makeClient();
  win.__STATSIG__ = { firstInstance: client };
  eq(client.getLayer("99999999").get("enable_i18n", false), false, "其它 layer");
});

check("重复调用幂等（第二次仍返回对象且值为 true）", () => {
  const { win } = makeEnv();
  const client = makeClient();
  win.__STATSIG__ = { firstInstance: client };
  const a = client.getLayer("72216192");
  const b = client.getLayer("72216192");
  if (!a || !b) throw new Error("某次调用返回了空值");
  eq(a.get("enable_i18n"), true, "第二次 enable_i18n");
});

check("__STATSIG__ 先存在、再被整体替换也能覆盖", () => {
  const { win } = makeEnv();
  const first = makeClient();
  win.__STATSIG__ = { firstInstance: first };
  const second = makeClient();
  win.__STATSIG__ = { firstInstance: second }; // 重新赋值
  eq(second.getLayer("72216192").get("enable_i18n", false), true, "替换后 enable_i18n");
});

check("_getLayerImpl 路径同样被覆盖且不丢返回值", () => {
  const { win } = makeEnv();
  const zh = makeLayer({ enable_i18n: false, locale_source: "SYSTEM" });
  const client = {
    _getLayerImpl(name) {
      return name === "72216192" ? zh : null;
    },
  };
  win.__STATSIG__ = { firstInstance: client };
  const r = client._getLayerImpl("72216192");
  if (!r) throw new Error("_getLayerImpl 返回了空值");
  eq(r.get("enable_i18n", false), true, "_getLayerImpl enable_i18n");
});

console.log("");
if (fails.length) {
  console.error(`FAILED: ${fails.length} 项未通过 / 共 ${pass + fails.length} 项`);
  for (const f of fails) console.error("  - " + f);
  process.exit(1);
}
console.log(`ALL PASS (${pass} 项)`);
