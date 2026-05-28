export default function(__VUE__, __WAILS_RUNTIME__) {
const { Fragment: e, Teleport: t, createBlock: n, createElementBlock: r, createElementVNode: i, createTextVNode: a, createVNode: o, defineComponent: s, onMounted: c, onUnmounted: l, openBlock: u, ref: d, renderList: f, resolveComponent: p, toDisplayString: m, withCtx: h, withDirectives: g } = __VUE__;
const { Events: _ } = __WAILS_RUNTIME__;
//#region ../views/classify/ClassifyPanel.vue?vue&type=script&setup=true&lang.ts
var v = { class: "explain-header" }, y = { class: "explain-actions" }, b = { class: "explain-panel" }, x = { class: "explain-scroll-inner" }, S = 10, C = /* @__PURE__ */ ((e, t) => {
	let n = e.__vccOpts || e;
	for (let [e, r] of t) n[e] = r;
	return n;
})(/* @__PURE__ */ s({
	__name: "ClassifyPanel",
	setup(s) {
		let C = [
			{
				value: "localAuthor",
				label: "本地作者"
			},
			{
				value: "siteAuthor",
				label: "站点作者"
			},
			{
				value: "localTag",
				label: "本地标签"
			},
			{
				value: "siteTag",
				label: "站点标签"
			},
			{
				value: "workName",
				label: "作品名称"
			},
			{
				value: "workSet",
				label: "作品集名称"
			},
			{
				value: "site",
				label: "站点名称"
			},
			{
				value: "unknown",
				label: "未知/无含义"
			}
		], w = new Set([
			"localAuthor",
			"localTag",
			"site"
		]), T = {
			mounted(e, t) {
				let n = function(n) {
					let r = n.target, i = r.scrollTop, a = r.clientHeight, o = r.scrollHeight;
					o <= a || o - i <= a + .5 && t.value(e.querySelector(".el-select__input")?.value || "");
				}, r = e.querySelector(".el-select__input")?.getAttribute("aria-controls"), i = r ? document.getElementById(r) : null;
				if (i) {
					let t = i.parentElement;
					t && (t.addEventListener("scroll", n), e.__ls_handleScroll = n, e.__ls_scrollDom = t);
				}
			},
			unmounted(e) {
				e.__ls_scrollDom && e.__ls_scrollDom.removeEventListener("scroll", e.__ls_handleScroll);
			}
		}, E = d(!1), D = d(null), O = d([]), k = d({});
		function A(e) {
			return w.has(e);
		}
		function j(e) {
			return k.value[e] || (k.value[e] = {
				options: [],
				pageNumber: 1,
				loading: !1,
				searchLoading: !1,
				hasMore: !0,
				currentQuery: ""
			}), k.value[e];
		}
		function M(e) {
			let t = window.__PLUGIN_CTX__?.custom?.apis;
			if (!t) return null;
			switch (e) {
				case "localAuthor": return t.localAuthorApi.localAuthorQuerySelectItemPageByName;
				case "localTag": return t.localTagApi.localTagQuerySelectItemPageByName;
				case "site": return t.siteApi.siteQuerySelectItemPageBySiteName;
				default: return null;
			}
		}
		async function N(e) {
			let t = O.value[e];
			if (!t) return;
			let n = j(e), r = M(t.type);
			if (!(!r || n.loading)) {
				n.loading = !0;
				try {
					let e = (await r({
						pageNumber: n.pageNumber,
						pageSize: S
					}, n.currentQuery))?.data || [];
					if (e.length > 0) {
						let t = e.filter((e) => e != null).map((e) => ({
							value: e.value,
							label: e.label
						}));
						n.options = [...n.options, ...t], n.pageNumber++;
					}
					n.hasMore = e.length >= S;
				} finally {
					n.loading = !1;
				}
			}
		}
		function P(e, t) {
			let n = j(e);
			n.currentQuery = t, n.pageNumber = 1, n.options = [], n.hasMore = !0, n.searchLoading = !0, N(e).finally(() => {
				n.searchLoading = !1;
			});
		}
		function F(e, t) {
			if (!t) return;
			let n = j(e);
			n.options.length === 0 && !n.loading && (n.pageNumber = 1, n.currentQuery = "", N(e));
		}
		function I(e, t) {
			e.name = j(t).options.find((t) => String(t.value) === String(e.id))?.label || "";
		}
		function L() {
			O.value.push({
				type: "unknown",
				id: "",
				name: D.value?.dirName || ""
			});
		}
		function R(e) {
			O.value.splice(e, 1), delete k.value[e];
		}
		function z(e, t) {
			e.id = "", e.name = A(e.type) ? "" : D.value?.dirName || "", delete k.value[t];
		}
		function B(e) {
			D.value = e, O.value = [{
				type: "unknown",
				id: "",
				name: e.dirName
			}], k.value = {}, E.value = !0;
		}
		function V() {
			D.value &&= (_.Emit("plugin:local-import:classify:response", {
				level: D.value.level,
				dirName: D.value.dirName,
				meanings: O.value.map((e) => ({
					...e,
					id: e.id == null ? "" : String(e.id)
				}))
			}), E.value = !1, null);
		}
		function H() {
			D.value &&= (_.Emit("plugin:local-import:classify:response", {
				level: D.value.level,
				dirName: D.value.dirName,
				cancel: !0
			}), E.value = !1, null);
		}
		let U = null;
		return c(() => {
			console.log("[ClassifyPanel] 已挂载，开始监听 classify:request 事件"), U = _.On("plugin:local-import:classify:request", (e) => {
				console.log("[ClassifyPanel] 收到分类请求:", e.data), B(e.data);
			});
		}), l(() => {
			U && U();
		}), (s, c) => {
			let l = p("el-tooltip"), d = p("el-button"), _ = p("el-text"), S = p("el-option"), w = p("el-select"), k = p("el-input"), M = p("el-scrollbar"), B = p("el-dialog");
			return u(), n(t, { to: "#dialog-mount-point" }, [o(B, {
				modelValue: E.value,
				"onUpdate:modelValue": c[0] ||= (e) => E.value = e,
				style: { height: "fit-content" },
				width: "520px",
				"close-on-click-modal": !1,
				"close-on-press-escape": !1,
				"show-close": !1
			}, {
				header: h(() => [i("div", v, [o(l, {
					content: "本地导入插件请求解释路径段的含义，以便为作品填充作者、标签等信息",
					placement: "bottom"
				}, {
					default: h(() => [...c[1] ||= [i("span", null, "解释路径含义", -1)]]),
					_: 1
				}), i("div", y, [
					o(d, {
						type: "success",
						icon: "CirclePlus",
						onClick: L
					}, {
						default: h(() => [...c[2] ||= [a("新增", -1)]]),
						_: 1
					}),
					o(d, {
						type: "primary",
						onClick: V
					}, {
						default: h(() => [...c[3] ||= [a("确定", -1)]]),
						_: 1
					}),
					o(d, { onClick: H }, {
						default: h(() => [...c[4] ||= [a("取消", -1)]]),
						_: 1
					})
				])])]),
				default: h(() => [i("div", b, [o(_, { class: "explain-path-text" }, {
					default: h(() => [a(m(D.value?.dirName), 1)]),
					_: 1
				}), o(M, { class: "explain-scroll" }, {
					default: h(() => [i("div", x, [(u(!0), r(e, null, f(O.value, (t, i) => (u(), r("div", {
						key: i,
						class: "explain-row"
					}, [
						o(w, {
							modelValue: t.type,
							"onUpdate:modelValue": (e) => t.type = e,
							class: "explain-type-select",
							onChange: (e) => z(t, i)
						}, {
							default: h(() => [(u(), r(e, null, f(C, (e) => o(S, {
								key: e.value,
								value: e.value,
								label: e.label
							}, null, 8, ["value", "label"])), 64))]),
							_: 1
						}, 8, [
							"modelValue",
							"onUpdate:modelValue",
							"onChange"
						]),
						A(t.type) ? g((u(), n(w, {
							key: 0,
							modelValue: t.id,
							"onUpdate:modelValue": (e) => t.id = e,
							class: "explain-name-input",
							filterable: "",
							remote: "",
							clearable: "",
							"remote-method": (e) => P(i, e),
							loading: j(i).searchLoading,
							onChange: (e) => I(t, i),
							onVisibleChange: (e) => F(i, e)
						}, {
							default: h(() => [(u(!0), r(e, null, f(j(i).options, (e) => (u(), n(S, {
								key: e.value,
								value: e.value,
								label: e.label
							}, null, 8, ["value", "label"]))), 128))]),
							_: 2
						}, 1032, [
							"modelValue",
							"onUpdate:modelValue",
							"remote-method",
							"loading",
							"onChange",
							"onVisibleChange"
						])), [[T, () => N(i)]]) : (u(), n(k, {
							key: 1,
							modelValue: t.name,
							"onUpdate:modelValue": (e) => t.name = e,
							class: "explain-name-input",
							clearable: ""
						}, null, 8, ["modelValue", "onUpdate:modelValue"])),
						o(d, {
							icon: "Remove",
							onClick: (e) => R(i)
						}, null, 8, ["onClick"])
					]))), 128))])]),
					_: 1
				})])]),
				_: 1
			}, 8, ["modelValue"])]);
		};
	}
}), [["__scopeId", "data-v-3598de69"]]);
//#endregion
return C;

}
