package panelserver

import "net/http"

const dashboardHTML = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>mini-sb 控制台</title><style>
:root{color-scheme:dark;--bg:#0b1020;--panel:#141b30;--line:#293351;--ink:#edf2ff;--muted:#aeb9d6;--accent:#66e3b4;--danger:#ff8394}*{box-sizing:border-box}body{margin:0;background:linear-gradient(135deg,#0b1020,#111a32);font:15px system-ui,sans-serif;color:var(--ink)}main{max-width:1080px;margin:40px auto;padding:0 20px}h1{margin:0;font-size:30px}p{color:var(--muted)}.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}.card{background:#141b30e8;border:1px solid var(--line);border-radius:14px;padding:20px}label{display:block;color:var(--muted);font-size:13px;margin:12px 0 5px}input,textarea{width:100%;background:#0c1325;border:1px solid var(--line);border-radius:8px;color:var(--ink);padding:10px;font:14px ui-monospace,monospace}textarea{min-height:190px;resize:vertical}button{margin-top:15px;border:0;border-radius:8px;background:var(--accent);color:#052218;font-weight:700;padding:10px 14px;cursor:pointer}pre{overflow:auto;background:#080d1b;border:1px solid var(--line);border-radius:8px;padding:12px;color:#c8d6ff;min-height:120px}.full{grid-column:1/-1}@media(max-width:760px){.grid{grid-template-columns:1fr}}
</style></head><body><main><h1>mini-sb 控制台</h1><p>原生控制面 MVP：登记 Agent、下发拓扑、查看 Agent 清单。Agent API 与管理 API 分离，流量上报使用幂等批次。</p>
<div class="grid"><section class="card"><h2>管理认证</h2><label>Admin Bearer Token</label><input id="token" type="password" autocomplete="off" placeholder="启动参数 -admin-token"><button onclick="listAgents()">刷新 Agent</button><pre id="agents">尚未加载</pre></section>
<section class="card"><h2>登记 Agent</h2><label>Agent ID</label><input id="agentID" placeholder="my-agent-01"><label>显示名称</label><input id="agentName" placeholder="My Edge Node"><label>Agent Secret</label><input id="agentSecret" type="password" placeholder="仅在创建/轮换时显示"><button onclick="createAgent()">创建 Agent</button><pre id="agentResult"></pre></section>
<section class="card full"><h2>下发 Topology</h2><label>目标 Agent ID</label><input id="topologyAgentID" placeholder="my-agent-01"><label>Topology JSON</label><textarea id="topology">{
  "nodes": [
    {
      "node_id": "42",
      "node_type": "vless",
      "outbound": { "type": "direct" }
    }
  ]
}</textarea><button onclick="putTopology()">保存并发布拓扑</button><pre id="topologyResult"></pre></section></div></main>
<script>
const headers=()=>({Authorization:'Bearer '+document.getElementById('token').value,'Content-Type':'application/json'});
async function api(path,opts={}){const r=await fetch(path,{...opts,headers:{...headers(),...(opts.headers||{})}});const t=await r.text();let b;try{b=JSON.parse(t)}catch{b=t}if(!r.ok)throw new Error(typeof b==='string'?b:JSON.stringify(b));return b}
async function listAgents(){try{document.getElementById('agents').textContent=JSON.stringify(await api('/api/v1/admin/agents'),null,2)}catch(e){document.getElementById('agents').textContent='错误: '+e.message}}
async function createAgent(){try{const b=await api('/api/v1/admin/agents',{method:'POST',body:JSON.stringify({id:agentID.value,name:agentName.value,secret:agentSecret.value})});agentResult.textContent=JSON.stringify(b,null,2);await listAgents()}catch(e){agentResult.textContent='错误: '+e.message}}
async function putTopology(){try{const topology=JSON.parse(document.getElementById('topology').value);const id=document.getElementById('topologyAgentID').value;topologyResult.textContent=JSON.stringify(await api('/api/v1/admin/agents/'+encodeURIComponent(id)+'/topology',{method:'PUT',body:JSON.stringify({topology})}),null,2)}catch(e){topologyResult.textContent='错误: '+e.message}}
</script></body></html>`

func (s *Server) dashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(dashboardHTML))
}
