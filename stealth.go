package gosurfer

import (
	"github.com/go-rod/rod/lib/proto"
)

// ApplyStealth injects anti-detection scripts into a page.
// Must be called before navigating to a URL. The scripts run before any
// page JavaScript, patching common bot-detection vectors.
func ApplyStealth(p *Page) error {
	_, err := p.rod.EvalOnNewDocument(stealthScript)
	return err
}

// stealthLaunchFlags returns Chrome flags that reduce detection surface.
func stealthLaunchFlags() map[string]string {
	return map[string]string{
		"disable-blink-features": "AutomationControlled",
		"excludeSwitches":        "enable-automation",
		"disable-infobars":       "",
	}
}

// defaultUserAgent returns a realistic Chrome user agent string.
func defaultUserAgent() string {
	return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
}

// applyStealthEmulation sets realistic device properties via CDP.
func applyStealthEmulation(p *Page) error {
	ua := defaultUserAgent()
	return proto.EmulationSetUserAgentOverride{
		UserAgent:      ua,
		AcceptLanguage: "en-US,en;q=0.9",
		Platform:       "macOS",
	}.Call(p.rod)
}

// stealthScript is injected before any page JS to evade bot detection.
// Ported from puppeteer-extra-plugin-stealth techniques.
const stealthScript = `
(() => {
	// 1. Remove navigator.webdriver flag (primary detection vector)
	Object.defineProperty(navigator, 'webdriver', {
		get: () => undefined,
		configurable: true
	});
	// Also delete it from the prototype
	delete Object.getPrototypeOf(navigator).webdriver;

	// 2. Emulate window.chrome object (missing in headless/automated Chrome)
	if (!window.chrome) {
		window.chrome = {};
	}
	if (!window.chrome.runtime) {
		window.chrome.runtime = {
			PlatformOs: {MAC:'mac',WIN:'win',ANDROID:'android',CROS:'cros',LINUX:'linux',OPENBSD:'openbsd'},
			PlatformArch: {ARM:'arm',X86_32:'x86-32',X86_64:'x86-64',MIPS:'mips',MIPS64:'mips64'},
			PlatformNaclArch: {ARM:'arm',X86_32:'x86-32',X86_64:'x86-64',MIPS:'mips',MIPS64:'mips64'},
			RequestUpdateCheckStatus: {THROTTLED:'throttled',NO_UPDATE:'no_update',UPDATE_AVAILABLE:'update_available'},
			OnInstalledReason: {INSTALL:'install',UPDATE:'update',CHROME_UPDATE:'chrome_update',SHARED_MODULE_UPDATE:'shared_module_update'},
			OnRestartRequiredReason: {APP_UPDATE:'app_update',OS_UPDATE:'os_update',PERIODIC:'periodic'},
			connect: function(){},
			sendMessage: function(){},
		};
	}
	if (!window.chrome.loadTimes) {
		window.chrome.loadTimes = function() {
			return {
				commitLoadTime: Date.now()/1000,
				connectionInfo: 'http/1.1',
				finishDocumentLoadTime: Date.now()/1000,
				finishLoadTime: Date.now()/1000,
				firstPaintAfterLoadTime: 0,
				firstPaintTime: Date.now()/1000,
				navigationType: 'Other',
				npnNegotiatedProtocol: 'unknown',
				requestTime: Date.now()/1000 - 0.16,
				startLoadTime: Date.now()/1000 - 0.16,
				wasAlternateProtocolAvailable: false,
				wasFetchedViaSpdy: false,
				wasNpnNegotiated: false
			};
		};
	}
	if (!window.chrome.csi) {
		window.chrome.csi = function() {
			return { onloadT: Date.now(), startE: Date.now()-100, pageT: 100, tran: 15 };
		};
	}

	// 3. Fake navigator.plugins (headless Chrome has none)
	const fakePluginData = [
		{name:'Chrome PDF Plugin', filename:'internal-pdf-viewer', description:'Portable Document Format',
		 mimeTypes:[{type:'application/x-google-chrome-pdf',suffixes:'pdf',description:'Portable Document Format'}]},
		{name:'Chrome PDF Viewer', filename:'mhjfbmdgcfjbbpaeojofohoefgiehjai', description:'',
		 mimeTypes:[{type:'application/pdf',suffixes:'pdf',description:''}]},
		{name:'Native Client', filename:'internal-nacl-plugin', description:'',
		 mimeTypes:[{type:'application/x-nacl',suffixes:'',description:'Native Client Executable'},
		            {type:'application/x-pnacl',suffixes:'',description:'Portable Native Client Executable'}]}
	];
	const pluginArray = Object.create(PluginArray.prototype);
	fakePluginData.forEach((pd, i) => {
		const plugin = Object.create(Plugin.prototype);
		Object.defineProperties(plugin, {
			name: {value: pd.name}, filename: {value: pd.filename},
			description: {value: pd.description}, length: {value: pd.mimeTypes.length}
		});
		pd.mimeTypes.forEach((mt, j) => {
			const mimeType = Object.create(MimeType.prototype);
			Object.defineProperties(mimeType, {
				type: {value: mt.type}, suffixes: {value: mt.suffixes},
				description: {value: mt.description}, enabledPlugin: {value: plugin}
			});
			Object.defineProperty(plugin, j, {value: mimeType, enumerable: true});
		});
		Object.defineProperty(pluginArray, i, {value: plugin, enumerable: true});
	});
	Object.defineProperty(pluginArray, 'length', {value: fakePluginData.length});
	Object.defineProperty(pluginArray, 'item', {value: function(i){return this[i]||null;}});
	Object.defineProperty(pluginArray, 'namedItem', {value: function(name){
		for(let i=0;i<this.length;i++){if(this[i].name===name)return this[i];}return null;
	}});
	Object.defineProperty(pluginArray, 'refresh', {value: function(){}});
	Object.defineProperty(navigator, 'plugins', {get:()=>pluginArray, configurable:true});

	// 4. navigator.languages
	Object.defineProperty(navigator, 'languages', {get:()=>['en-US','en'], configurable:true});

	// 5. Permissions API fix (notification permission quirk)
	if (navigator.permissions) {
		const origQuery = navigator.permissions.query.bind(navigator.permissions);
		navigator.permissions.query = (params) => {
			if (params.name === 'notifications') {
				return Promise.resolve({state: Notification.permission, onchange: null});
			}
			return origQuery(params);
		};
	}

	// 6. Fix window outer dimensions (0 in headless)
	if (window.outerWidth === 0) {
		Object.defineProperty(window, 'outerWidth', {get:()=>window.innerWidth, configurable:true});
	}
	if (window.outerHeight === 0) {
		Object.defineProperty(window, 'outerHeight', {get:()=>window.innerHeight+85, configurable:true});
	}

	// 7. Hardware concurrency (1 in many headless configs)
	if (navigator.hardwareConcurrency <= 1) {
		Object.defineProperty(navigator, 'hardwareConcurrency', {get:()=>4, configurable:true});
	}

	// 8. Device memory
	if (!navigator.deviceMemory || navigator.deviceMemory < 4) {
		Object.defineProperty(navigator, 'deviceMemory', {get:()=>8, configurable:true});
	}

	// 9. WebGL vendor/renderer spoofing (prevents headless fingerprinting)
	const origGetParameter = WebGLRenderingContext.prototype.getParameter;
	WebGLRenderingContext.prototype.getParameter = function(param) {
		const debugInfo = this.getExtension('WEBGL_debug_renderer_info');
		if (debugInfo) {
			if (param === debugInfo.UNMASKED_VENDOR_WEBGL) return 'Intel Inc.';
			if (param === debugInfo.UNMASKED_RENDERER_WEBGL) return 'Intel Iris OpenGL Engine';
		}
		return origGetParameter.call(this, param);
	};
	if (typeof WebGL2RenderingContext !== 'undefined') {
		const origGetParameter2 = WebGL2RenderingContext.prototype.getParameter;
		WebGL2RenderingContext.prototype.getParameter = function(param) {
			const debugInfo = this.getExtension('WEBGL_debug_renderer_info');
			if (debugInfo) {
				if (param === debugInfo.UNMASKED_VENDOR_WEBGL) return 'Intel Inc.';
				if (param === debugInfo.UNMASKED_RENDERER_WEBGL) return 'Intel Iris OpenGL Engine';
			}
			return origGetParameter2.call(this, param);
		};
	}

	// 10. Fix media devices (empty in headless)
	if (navigator.mediaDevices && navigator.mediaDevices.enumerateDevices) {
		const origEnum = navigator.mediaDevices.enumerateDevices.bind(navigator.mediaDevices);
		navigator.mediaDevices.enumerateDevices = async function() {
			const devices = await origEnum();
			if (devices.length === 0) {
				return [
					{deviceId:'default',kind:'audioinput',label:'',groupId:'default'},
					{deviceId:'default',kind:'audiooutput',label:'',groupId:'default'},
					{deviceId:'',kind:'videoinput',label:'',groupId:''}
				];
			}
			return devices;
		};
	}

	// 11. Connection type (missing in headless)
	if (navigator.connection) {
		Object.defineProperty(navigator.connection, 'rtt', {get:()=>50, configurable:true});
	}

	// 12. Fix toString on overridden functions (detection vector)
	const origToString = Function.prototype.toString;
	const nativePatterns = new Map();
	function patchToString(fn, name) {
		nativePatterns.set(fn, 'function ' + name + '() { [native code] }');
	}
	Function.prototype.toString = function() {
		if (nativePatterns.has(this)) return nativePatterns.get(this);
		return origToString.call(this);
	};
	patchToString(Function.prototype.toString, 'toString');
})();
`
