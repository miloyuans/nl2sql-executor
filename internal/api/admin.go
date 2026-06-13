package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"nl2sql-executor-go-prod/internal/job"
)

type adminSQLExecuteRequest struct {
	RequestID    string `json:"request_id"`
	UserID       string `json:"user_id"`
	ChatID       string `json:"chat_id"`
	Question     string `json:"question"`
	DatasourceID string `json:"data_source_id"`
	SQL          string `json:"sql"`
}

func (s *Server) adminIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	if _, ok := s.currentAdminUser(r); !ok {
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(adminHTML))
}

func (s *Server) adminJobs(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	result := s.mgr.List(job.ListOptions{
		Status: r.URL.Query().Get("status"),
		Query:  r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: offset,
	})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) adminJobAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/admin/jobs/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, errResp("missing job id"))
		return
	}
	if strings.HasSuffix(path, "/rerun") {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
			return
		}
		id := strings.TrimSuffix(path, "/rerun")
		id = strings.Trim(id, "/")
		j, err := s.mgr.Rerun(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, errResp("job not found"))
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job_id": j.ID, "status": j.Status})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	id := strings.Trim(path, "/")
	j, ok := s.mgr.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errResp("job not found"))
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) adminSQLExecute(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAPI(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errResp("method not allowed"))
		return
	}
	defer r.Body.Close()
	var req adminSQLExecuteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid json: "+err.Error()))
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		writeJSON(w, http.StatusBadRequest, errResp("sql is required"))
		return
	}
	chatID := strings.TrimSpace(req.ChatID)
	userID := strings.TrimSpace(req.UserID)
	if chatID == "" {
		chatID = userID
	}
	j, err := s.mgr.Submit(job.QueryRequest{
		RequestID:    req.RequestID,
		UserID:       userID,
		ChatID:       chatID,
		Question:     req.Question,
		DatasourceID: req.DatasourceID,
		SQL:          req.SQL,
	})
	if err != nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"job_id": j.ID, "status": j.Status, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": j.ID, "status": j.Status})
}

func adminLoginHTML(ssoEnabled bool) string {
	sso := ""
	if ssoEnabled {
		sso = `<a class="sso" href="/admin/sso/login">使用 OIDC / SSO 登录</a><div class="or">或使用本地管理员登录</div>`
	}
	return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>OpenClaw Admin Login</title><style>*{box-sizing:border-box}html,body{margin:0;min-height:100%;background:#07111f;color:#e5edf7;font:14px/1.45 Arial,"Microsoft YaHei",sans-serif}body{display:grid;place-items:center}.card{width:min(440px,calc(100vw - 32px));background:#0e1c30;border:1px solid #24405f;border-radius:16px;padding:28px;box-shadow:0 24px 80px rgba(0,0,0,.35)}h1{margin:0 0 4px;font-size:25px}.sub{color:#91a8c3;margin-bottom:22px}.field{display:grid;gap:7px;margin:14px 0}.field span{color:#b9c8da;font-weight:700}.field input{height:42px;border:1px solid #2a4668;background:#07111f;color:#e5edf7;border-radius:9px;padding:0 12px}button,.sso{height:42px;width:100%;border:0;border-radius:9px;background:linear-gradient(135deg,#19a8ff,#7c5cff);color:white;font-weight:900;cursor:pointer;margin-top:10px;text-decoration:none;display:grid;place-items:center}.sso{background:#10223a;border:1px solid #2d4b70}.or{text-align:center;color:#8fa4bd;font-size:12px;margin:12px 0 2px}.msg{display:none;border:1px solid #a83a4b;background:#2b1220;color:#ffd7df;border-radius:8px;padding:10px;margin-bottom:12px}.hint{margin-top:16px;color:#71859f;font-size:12px}</style></head><body><main class="card"><h1>OpenClaw NL2SQL</h1><div class="sub">管理后台登录</div><div class="msg" id="msg"></div>` + sso + `<label class="field"><span>用户名</span><input id="u" autocomplete="username" autofocus value="admin"></label><label class="field"><span>密码</span><input id="p" type="password" autocomplete="current-password" placeholder="默认 admin"></label><button onclick="login()">登录</button><div class="hint">默认本地管理员：admin / admin。登录会在重启后失效，默认 12 小时自动过期。</div></main><script>
async function login(){let msg=document.getElementById('msg');msg.style.display='none';try{let r=await fetch('/v1/admin/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:document.getElementById('u').value,password:document.getElementById('p').value})});let j=await r.json().catch(function(){return {error:'invalid json'}});if(!r.ok)throw new Error(j.error||'登录失败');location.href='/admin'}catch(e){msg.textContent=e.message;msg.style.display='block'}}
document.addEventListener('keydown',function(e){if(e.key==='Enter')login()});
</script></body></html>`
}

const adminHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>OpenClaw NL2SQL Admin</title>
<style>
:root{--bg:#07111f;--side:#050d18;--panel:#0e1c30;--panel2:#0a1626;--line:#213954;--muted:#8fa4bd;--text:#e5edf7;--accent:#24a7ff;--ok:#23c16b;--warn:#ffb020;--danger:#ff5f78;--violet:#7c5cff;--code:#050d18}*{box-sizing:border-box}html,body{margin:0;min-height:100%;background:var(--bg);color:var(--text);font:14px/1.45 Arial,"Microsoft YaHei",sans-serif}button,input,select,textarea{font:inherit}.app{display:grid;grid-template-columns:226px minmax(0,1fr);min-height:100vh}.side{background:var(--side);border-right:1px solid var(--line);padding:18px 10px}.brand{display:flex;gap:10px;align-items:center;margin:0 0 18px}.mark{width:40px;height:40px;border-radius:12px;background:linear-gradient(135deg,#2ac7ff,#7c5cff);display:grid;place-items:center;font-weight:900}.brand strong{display:block;font-size:18px}.brand span{color:var(--muted);font-size:12px}.nav{display:grid;gap:6px}.nav button{height:38px;text-align:left;border:1px solid transparent;background:transparent;color:#c7d6e8;border-radius:9px;padding:0 12px;cursor:pointer}.nav button.active,.nav button:hover{background:#10223a;border-color:#2a4d75;color:#fff}.main{min-width:0}.top{height:62px;border-bottom:1px solid var(--line);display:flex;align-items:center;justify-content:space-between;padding:0 20px;background:#091421;position:sticky;top:0;z-index:5}.title h1{margin:0;font-size:22px}.title div{color:var(--muted);font-size:12px}.top-actions{display:flex;align-items:center;gap:10px}.pill{border:1px solid #1f6c45;background:#092b20;color:#6ff0a6;border-radius:999px;padding:7px 12px;font-weight:800}.user{border:1px solid var(--line);background:#101d31;border-radius:999px;padding:6px 12px;display:flex;gap:10px;align-items:center}.avatar{width:28px;height:28px;border-radius:50%;background:#6c63ff;display:grid;place-items:center;font-weight:900}.btn{height:34px;border:1px solid #2d4b70;background:#10223a;color:#eaf4ff;border-radius:8px;padding:0 12px;font-weight:800;cursor:pointer;text-decoration:none;display:inline-flex;align-items:center;justify-content:center}.btn.primary{background:linear-gradient(135deg,#19a8ff,#7c5cff);border:0}.btn.danger{border-color:#8e2a3d;background:#2b1220;color:#ffd7df}.btn.ok{border-color:#1f6c45;background:#092b20;color:#6ff0a6}.btn:disabled{opacity:.45;cursor:not-allowed}.content{padding:16px 22px 22px}.cards{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;margin-bottom:14px}.metric{background:#0a1728;border:1px solid var(--line);border-radius:12px;padding:14px}.metric .label{color:#9cb0c7;font-size:12px}.metric .value{font-size:24px;font-weight:900;margin-top:6px}.panel{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:16px}.panel h2{margin:0 0 4px;font-size:18px}.panel-sub{color:var(--muted);font-size:12px;margin-bottom:14px}.grid{display:grid;grid-template-columns:minmax(520px,1fr) minmax(420px,.7fr);gap:14px;align-items:start}.toolbar{display:flex;flex-wrap:wrap;gap:8px;align-items:center;margin-bottom:12px}.input,.select,input,select,textarea{border:1px solid #2d4b70;background:#081322;color:#e5edf7;border-radius:8px;padding:0 10px}input,select{height:34px}textarea{width:100%;min-height:260px;padding:12px;resize:vertical}.mono,pre{font-family:Consolas,Monaco,monospace}.table-wrap{overflow:auto;max-height:calc(100vh - 260px);border:1px solid var(--line);border-radius:10px}.table{width:100%;border-collapse:collapse}.table th,.table td{border-bottom:1px solid var(--line);padding:10px;text-align:left;vertical-align:top}.table th{color:#b8c9df;background:#0a1728;position:sticky;top:0}.badge{display:inline-flex;align-items:center;border-radius:999px;padding:3px 8px;font-size:12px;font-weight:800;border:1px solid #2f4968}.badge.sent,.badge.sent_cached,.badge.active{background:#07351f;color:#75f0a4;border-color:#1b7d4b}.badge.failed,.badge.rejected{background:#351321;color:#ff9caf;border-color:#7a2b3c}.badge.queued,.badge.running,.badge.validating{background:#35260a;color:#ffd06f;border-color:#8a6518}.hint,.meta{color:#9cb0c7;font-size:12px}.ellipsis{max-width:360px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.hidden{display:none!important}pre{white-space:pre-wrap;word-break:break-word;margin:0;background:var(--code);color:#dbeafe;border:1px solid #213954;border-radius:10px;padding:12px;max-height:340px;overflow:auto}.row{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:10px}.field{display:grid;gap:6px}.field label{color:#b8c9df;font-weight:700;font-size:12px}.toast{position:fixed;right:18px;bottom:18px;background:#0e1c30;border:1px solid #2d4b70;color:#fff;border-radius:12px;padding:12px 14px;box-shadow:0 16px 60px rgba(0,0,0,.35);display:none;max-width:560px;z-index:20}.event{border-left:3px solid #2a4d75;padding:7px 9px;margin:8px 0;background:#0a1728;border-radius:0 8px 8px 0}.split{display:grid;grid-template-columns:1fr 1fr;gap:14px}.download{color:#8bd3ff}.setting-grid{display:grid;grid-template-columns:minmax(420px,.9fr) minmax(420px,1fr);gap:14px}.switchline{display:flex;gap:10px;align-items:center;min-height:34px}.switchline input{width:18px;height:18px}.form-actions{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}@media(max-width:1120px){.app{grid-template-columns:1fr}.side{position:sticky;top:0;z-index:10}.nav{grid-template-columns:repeat(5,1fr)}.grid,.setting-grid,.split,.row{grid-template-columns:1fr}.cards{grid-template-columns:repeat(2,1fr)}}
</style>
</head>
<body>
<div class="app">
  <aside class="side"><div class="brand"><div class="mark">SQL</div><div><strong>OpenClaw</strong><span>NL2SQL Admin</span></div></div><nav class="nav"><button class="active" data-page="manual" onclick="showPage('manual')">手动查询</button><button data-page="jobs" onclick="showPage('jobs')">任务列表</button><button data-page="export" onclick="showPage('export')">数据导出</button><button data-page="settings" onclick="showPage('settings')">系统设置</button><button data-page="users" onclick="showPage('users')">用户管理</button></nav></aside>
  <main class="main"><header class="top"><div class="title"><h1 id="pageTitle">手动查询</h1><div id="pageSub">直接输入 SQL 执行，支持填写用户/Chat ID 后私聊发送结果。</div></div><div class="top-actions"><span class="pill" id="health">Ready</span><div class="user"><div class="avatar">OC</div><span id="me">加载中</span></div><button class="btn" onclick="logout()">退出</button></div></header>
  <section class="content">
    <div class="cards"><div class="metric"><div class="label">任务总数</div><div class="value" id="mJobs">-</div></div><div class="metric"><div class="label">成功</div><div class="value" id="mOK">-</div></div><div class="metric"><div class="label">失败</div><div class="value" id="mFail">-</div></div><div class="metric"><div class="label">数据源</div><div class="value" id="mDS">-</div></div></div>

    <div id="page-manual" class="panel"><h2>手动查询</h2><div class="panel-sub">用于排查 OpenClaw 生成 SQL 结果为 0、字段不存在、路由错误等问题。</div><div class="row"><div class="field"><label>数据源</label><select id="mDatasource"><option value="">自动路由</option></select></div><div class="field"><label>发送到用户/Chat ID</label><input id="mChat" placeholder="为空则只在后台查看；填写后私聊发送"></div></div><div class="row"><div class="field"><label>问题描述</label><input id="mQuestion" placeholder="例如：手动排查昨日美国VPBET充值提现"></div><div class="field"><label>Request ID（可选）</label><input id="mRequest" placeholder="为空自动生成"></div></div><div class="field"><label>SQL</label><textarea id="mSQL" class="mono" placeholder="SELECT ..."></textarea></div><div class="form-actions"><button class="btn primary" onclick="executeManual()">执行查询</button><button class="btn" onclick="mSQL.value=''">清空</button><button class="btn ok" onclick="fillDebugSQL()">填入排查模板</button></div><p class="hint" id="mResult"></p></div>

    <div id="page-jobs" class="grid hidden"><div class="panel"><h2>任务列表</h2><div class="panel-sub">查看所有任务事件、SQL、结果；可以点击重跑并发送给原用户。</div><div class="toolbar"><input id="q" placeholder="搜索任务ID / 用户 / SQL / 错误 / 结果" onkeydown="if(event.key==='Enter')loadJobs()"><select id="status"><option value="">全部状态</option><option value="queued">queued</option><option value="validating">validating</option><option value="running">running</option><option value="sent">sent</option><option value="sent_cached">sent_cached</option><option value="failed">failed</option><option value="rejected">rejected</option></select><select id="limit"><option>20</option><option selected>50</option><option>100</option><option>200</option></select><button class="btn primary" onclick="loadJobs()">刷新</button></div><div class="hint" id="jobCount">加载中...</div><div class="table-wrap"><table class="table"><thead><tr><th>时间</th><th>状态</th><th>任务</th><th>用户</th><th>问题/SQL</th><th>结果</th><th>操作</th></tr></thead><tbody id="jobsBody"></tbody></table></div></div><div class="panel"><h2>任务详情</h2><div id="detailEmpty" class="hint">点击左侧“详情”查看 SQL、结果和事件。</div><div id="detailBox" class="hidden"><div><span id="dStatus" class="badge"></span> <span class="badge" id="dID"></span> <span class="badge" id="dDS"></span></div><div class="field" style="margin-top:12px"><label>问题</label><div id="dQuestion" class="hint"></div></div><div class="field"><label>执行 SQL</label><pre id="dSQL"></pre></div><div class="field"><label>查询结果</label><pre id="dResult"></pre></div><div class="field"><label>结果预览</label><div id="dRows" class="table-wrap"></div></div><div class="field"><label>事件记录</label><div id="dEvents"></div></div><div class="form-actions"><button class="btn primary" onclick="rerunCurrent()">重新执行并发送给原用户</button><button class="btn" onclick="copySQL()">复制 SQL</button></div></div></div></div>

    <div id="page-export" class="split hidden"><div class="panel"><h2>数据导出</h2><div class="panel-sub">一键导出当前账号可访问的库、表、视图、索引、字段和备注，输出 JSON + Markdown，供 AI 学习最新数据结构。</div><div class="field"><label>数据源</label><select id="eDatasource"><option value="">默认数据源</option></select></div><div class="switchline"><input id="eSystem" type="checkbox"><label for="eSystem">包含系统库 information_schema / mysql / sys 等</label></div><div class="form-actions"><button class="btn primary" onclick="exportSchema()">一键导出架构数据</button><button class="btn" onclick="loadExports()">刷新导出文件</button></div><pre id="eResult" style="margin-top:12px">等待导出...</pre></div><div class="panel"><h2>导出文件</h2><div class="panel-sub">建议将 Markdown 文件投喂给 OpenClaw，JSON 文件用于程序化 RAG 更新。</div><div class="table-wrap"><table class="table"><thead><tr><th>文件</th><th>大小</th><th>时间</th><th>操作</th></tr></thead><tbody id="exportsBody"></tbody></table></div></div></div>

    <div id="page-settings" class="setting-grid hidden"><div class="panel"><h2>系统设置</h2><div class="panel-sub">默认关闭 SSO；开启后走 OIDC 登录，本地账号保留为应急入口。</div><div class="switchline"><input id="ssoEnabled" type="checkbox"><label for="ssoEnabled">启用 SSO / OIDC</label></div><div class="field"><label>Issuer URL</label><input id="ssoIssuer" placeholder="https://keycloak.example.com/realms/openclaw"></div><div class="field"><label>Client ID</label><input id="ssoClient"></div><div class="field"><label>Client Secret</label><input id="ssoSecret" type="password" placeholder="留空则保持原密钥"></div><div class="field"><label>Redirect URL</label><input id="ssoRedirect" placeholder="https://your-domain/admin/sso/callback"></div><div class="field"><label>Scopes</label><input id="ssoScopes" placeholder="openid profile email"></div><div class="row"><div class="field"><label>管理员用户白名单，逗号分隔</label><input id="ssoAdmins"></div><div class="field"><label>管理员角色，逗号分隔</label><input id="ssoRoles"></div></div><div class="form-actions"><button class="btn primary" onclick="saveSettings()">保存设置</button><button class="btn" onclick="loadSettings()">重新加载</button></div></div><div class="panel"><h2>输出与导出配置</h2><div class="switchline"><input id="tgCompact" type="checkbox"><label for="tgCompact">Telegram 仅输出查询语句和结果</label></div><div class="switchline"><input id="tgCSV" type="checkbox"><label for="tgCSV">自动发送 CSV</label></div><div class="switchline"><input id="tgSVG" type="checkbox"><label for="tgSVG">自动发送图表 SVG</label></div><div class="field"><label>最大内联结果行数</label><input id="tgRows" type="number" min="1"></div><div class="field"><label>架构导出目录</label><input id="schemaDir"></div><div class="field"><label>单类元数据最大行数</label><input id="schemaMaxRows" type="number" min="1000"></div><pre id="settingsRaw">加载中...</pre></div></div>

    <div id="page-users" class="split hidden"><div class="panel"><h2>用户管理</h2><div class="panel-sub">用于本地管理后台登录；SSO 用户由身份提供商管理。</div><div class="row"><div class="field"><label>用户名</label><input id="uName"></div><div class="field"><label>显示名</label><input id="uDisplay"></div></div><div class="row"><div class="field"><label>角色</label><select id="uRole"><option value="admin">admin</option><option value="user">user</option></select></div><div class="field"><label>状态</label><select id="uStatus"><option value="active">active</option><option value="disabled">disabled</option></select></div></div><div class="field"><label>密码</label><input id="uPass" type="password" placeholder="新增必填；编辑留空则不改"></div><div class="form-actions"><button class="btn primary" onclick="saveUser()">保存用户</button><button class="btn" onclick="clearUserForm()">清空</button></div></div><div class="panel"><h2>本地用户</h2><div class="table-wrap"><table class="table"><thead><tr><th>用户名</th><th>角色</th><th>状态</th><th>来源</th><th>操作</th></tr></thead><tbody id="usersBody"></tbody></table></div></div></div>
  </section></main>
</div><div class="toast" id="toast"></div>
<script>
let currentJobID='';let datasources=[];const titles={manual:['手动查询','直接输入 SQL 执行，支持填写用户/Chat ID 后私聊发送结果。'],jobs:['任务列表','查看所有任务事件、SQL、结果；支持重跑并发送给原用户。'],export:['数据导出','一键导出库表、视图、索引、字段和备注，供 AI 更新学习。'],settings:['系统设置','配置输出格式、架构导出、SSO / OIDC。'],users:['用户管理','管理本地后台用户；SSO 用户由身份提供商维护。']};
function esc(s){return (s==null?'':String(s)).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
function fmtTime(s){if(!s)return '';try{return new Date(s).toLocaleString()}catch(e){return s}}
function fmtSize(n){n=Number(n||0);if(n>1048576)return (n/1048576).toFixed(1)+' MB';if(n>1024)return (n/1024).toFixed(1)+' KB';return n+' B'}
function toast(s){let t=document.getElementById('toast');t.textContent=s;t.style.display='block';setTimeout(function(){t.style.display='none'},3600)}
async function api(path,opt){let r=await fetch(path,opt||{});if(r.status===401){location.href='/admin/login';return {}}let j=await r.json().catch(function(){return {error:'invalid json'}});if(!r.ok)throw new Error(j.error||r.statusText);return j}
function showPage(p){document.querySelectorAll('[id^="page-"]').forEach(function(x){x.classList.add('hidden')});document.getElementById('page-'+p).classList.remove('hidden');document.querySelectorAll('.nav button').forEach(function(b){b.classList.toggle('active',b.dataset.page===p)});pageTitle.textContent=titles[p][0];pageSub.textContent=titles[p][1];if(p==='jobs')loadJobs();if(p==='export')loadExports();if(p==='settings')loadSettings();if(p==='users')loadUsers()}
async function loadMe(){let data=await api('/v1/admin/auth/me');me.textContent=(data.user&&data.user.username)||'anonymous';if(!data.auth_enabled)document.querySelector('.top-actions .btn').style.display='none'}
async function loadDatasources(){try{let data=await api('/v1/datasources');datasources=data.datasources||[];mDS.textContent=datasources.length;['mDatasource','eDatasource'].forEach(function(id){let sel=document.getElementById(id);datasources.forEach(function(d){let o=document.createElement('option');o.value=d.id;o.textContent=d.id+(d.default?'（默认）':'')+' - '+(d.description||'');sel.appendChild(o)})})}catch(e){}}
async function loadJobs(){try{let qs=new URLSearchParams({q:q.value,status:status.value,limit:limit.value});let data=await api('/v1/admin/jobs?'+qs.toString());let arr=data.jobs||[];mJobs.textContent=data.total;mOK.textContent=arr.filter(function(x){return String(x.status).startsWith('sent')}).length;mFail.textContent=arr.filter(function(x){return x.status==='failed'||x.status==='rejected'}).length;jobCount.textContent='共 '+data.total+' 条，当前显示 '+arr.length+' 条';let html='';arr.forEach(function(j){let sql=j.rewritten_sql||j.sql||'';let title=j.question||sql;html+='<tr><td>'+esc(fmtTime(j.created_at))+'</td><td><span class="badge '+esc(j.status)+'">'+esc(j.status)+'</span></td><td class="mono">'+esc(j.id)+'</td><td>'+esc(j.chat_id||j.user_id||'')+'</td><td><div class="ellipsis" title="'+esc(title)+'">'+esc(title)+'</div>'+(j.error?'<div class="hint" style="color:#ff9caf">'+esc(j.error)+'</div>':'')+'</td><td>'+esc(j.result_row_count||0)+' 行<br><span class="hint">'+esc(j.result_duration_ms||0)+' ms</span></td><td><button class="btn" onclick="loadDetail(\''+esc(j.id)+'\')">详情</button> <button class="btn primary" onclick="rerun(\''+esc(j.id)+'\')">重跑</button></td></tr>'});jobsBody.innerHTML=html||'<tr><td colspan="7" class="hint">暂无记录</td></tr>'}catch(e){toast(e.message)}}
async function loadDetail(id){try{let j=await api('/v1/admin/jobs/'+encodeURIComponent(id));currentJobID=j.id;detailEmpty.classList.add('hidden');detailBox.classList.remove('hidden');dStatus.className='badge '+esc(j.status);dStatus.textContent=j.status;dID.textContent=j.id;dDS.textContent=j.data_source_id||(j.request&&j.request.data_source_id)||'自动路由';dQuestion.textContent=(j.request&&j.request.question)||'';dSQL.textContent=j.rewritten_sql||(j.request&&j.request.sql)||'';dResult.textContent=j.result_text||j.error||'暂无结果';renderRows(j);renderEvents(j.events||[])}catch(e){toast(e.message)}}
function renderRows(j){let cols=j.result_columns||[],rows=j.result_rows||[];if(!cols.length){dRows.innerHTML='<div class="hint" style="padding:10px">暂无行预览</div>';return}let html='<table class="table"><thead><tr>'+cols.map(function(c){return '<th>'+esc(c)+'</th>'}).join('')+'</tr></thead><tbody>';rows.forEach(function(r){html+='<tr>'+cols.map(function(c,i){return '<td>'+esc(r[i]||'')+'</td>'}).join('')+'</tr>'});html+='</tbody></table>';if((j.result_row_count||0)>rows.length)html+='<div class="hint">仅展示前 '+rows.length+' 行 / 总 '+j.result_row_count+' 行</div>';dRows.innerHTML=html}
function renderEvents(events){dEvents.innerHTML=events.map(function(e){return '<div class="event"><b>'+esc(e.type)+'</b> <span class="hint">'+esc(fmtTime(e.at))+'</span><br>'+esc(e.message||'')+'</div>'}).join('')||'<div class="hint">暂无事件</div>'}
async function rerun(id){try{let data=await api('/v1/admin/jobs/'+encodeURIComponent(id)+'/rerun',{method:'POST'});toast('已提交重跑任务：'+data.job_id);showPage('jobs');setTimeout(function(){loadDetail(data.job_id)},800)}catch(e){toast(e.message)}}
function rerunCurrent(){if(currentJobID)rerun(currentJobID)}
function copySQL(){navigator.clipboard&&navigator.clipboard.writeText(dSQL.textContent);toast('SQL 已复制')}
async function executeManual(){try{let payload={data_source_id:mDatasource.value,chat_id:mChat.value,user_id:mChat.value,question:mQuestion.value,request_id:mRequest.value,sql:mSQL.value};let data=await api('/v1/admin/sql/execute',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});mResult.textContent='已提交任务：'+data.job_id+'；若填写用户ID，完成后会私聊发送。';showPage('jobs');setTimeout(function(){loadDetail(data.job_id)},900)}catch(e){toast(e.message)}}
function fillDebugSQL(){mSQL.value="SELECT *\nFROM information_schema.columns\nWHERE table_schema = 'international-data'\n  AND table_name = 'area_pay_statistics'\nLIMIT 50"}
async function exportSchema(){try{eResult.textContent='正在导出，请稍候...';let data=await api('/v1/admin/schema/export',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({data_source_id:eDatasource.value,include_system_schemas:eSystem.checked})});renderExportResult(data);loadExports()}catch(e){eResult.textContent=e.message;toast(e.message)}}
function renderExportResult(data){let s=data.summary||{};let html='导出完成：库 '+esc(s.database_count||0)+'，表 '+esc(s.table_count||0)+'，视图 '+esc(s.view_count||0)+'，字段 '+esc(s.column_count||0)+'，索引 '+esc(s.index_count||0)+'。';html+='\nJSON：'+esc(data.json_file||'')+'\nMarkdown：'+esc(data.markdown_file||'');if(data.errors&&data.errors.length)html+='\n警告：'+esc(data.errors.join('；'));html+='\n\n可在右侧文件列表点击“下载”保存到本地。';eResult.textContent=html}
async function downloadExport(file){try{let name=String(file||'');if(!name)return;let r=await fetch('/v1/admin/schema/download?file='+encodeURIComponent(name),{credentials:'same-origin'});if(r.status===401){location.href='/admin/login';return}if(!r.ok){let t=await r.text();throw new Error(t||('下载失败 '+r.status))}let blob=await r.blob();let url=URL.createObjectURL(blob);let a=document.createElement('a');a.href=url;a.download=name;document.body.appendChild(a);a.click();a.remove();setTimeout(function(){URL.revokeObjectURL(url)},1500);toast('已开始下载：'+name)}catch(e){toast(e.message)}}
async function loadExports(){try{let data=await api('/v1/admin/schema/exports');exportsBody.innerHTML=(data.exports||[]).map(function(f){return '<tr><td class="mono">'+esc(f.file)+'</td><td>'+fmtSize(f.size)+'</td><td>'+fmtTime(f.updated_at)+'</td><td><button class="btn" data-file="'+esc(f.file)+'" onclick="downloadExport(this.dataset.file)">下载</button></td></tr>'}).join('')||'<tr><td colspan="4" class="hint">暂无导出文件</td></tr>'}catch(e){}}
async function loadSettings(){try{let s=await api('/v1/admin/settings');ssoEnabled.checked=!!s.sso.enabled;ssoIssuer.value=s.sso.issuer_url||'';ssoClient.value=s.sso.client_id||'';ssoRedirect.value=s.sso.redirect_url||'';ssoScopes.value=s.sso.scopes||'openid profile email';ssoAdmins.value=(s.sso.admin_users||[]).join(',');ssoRoles.value=(s.sso.admin_roles||[]).join(',');tgCompact.checked=!!s.telegram.compact_result_only;tgCSV.checked=!!s.telegram.send_csv;tgSVG.checked=!!s.telegram.send_chart_svg;tgRows.value=s.telegram.max_inline_rows||30;schemaDir.value=s.schema_export.dir||'';schemaMaxRows.value=s.schema_export.max_rows||200000;settingsRaw.textContent=JSON.stringify(s,null,2)}catch(e){toast(e.message)}}
function listVal(s){return String(s||'').split(',').map(function(x){return x.trim()}).filter(Boolean)}
async function saveSettings(){try{let payload={keep_client_secret:true,sso:{enabled:ssoEnabled.checked,issuer_url:ssoIssuer.value,client_id:ssoClient.value,client_secret:ssoSecret.value,redirect_url:ssoRedirect.value,scopes:ssoScopes.value,admin_users:listVal(ssoAdmins.value),admin_roles:listVal(ssoRoles.value),user_roles:['user','openclaw-user']},telegram:{compact_result_only:tgCompact.checked,send_csv:tgCSV.checked,send_chart_svg:tgSVG.checked,max_inline_rows:Number(tgRows.value||30)},schema_export:{dir:schemaDir.value,max_rows:Number(schemaMaxRows.value||200000),include_system_schemas:false,system_schemas:['information_schema','mysql','performance_schema','sys','__internal_schema']}};let data=await api('/v1/admin/settings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});settingsRaw.textContent=JSON.stringify(data,null,2);ssoSecret.value='';toast('设置已保存到运行时配置；如需持久化，请同步更新 config.yaml')}catch(e){toast(e.message)}}
async function loadUsers(){try{let data=await api('/v1/admin/users');usersBody.innerHTML=(data.users||[]).map(function(u){return '<tr><td>'+esc(u.username)+'</td><td>'+esc(u.role)+'</td><td><span class="badge '+esc(u.status)+'">'+esc(u.status)+'</span></td><td>'+esc(u.source)+'</td><td><button class="btn" onclick="editUser(\''+esc(u.username)+'\',\''+esc(u.display_name||'')+'\',\''+esc(u.role)+'\',\''+esc(u.status)+'\')">编辑</button> <button class="btn danger" onclick="deleteUser(\''+esc(u.username)+'\')">删除</button></td></tr>'}).join('')||'<tr><td colspan="5" class="hint">暂无用户</td></tr>'}catch(e){toast(e.message)}}
function editUser(a,b,c,d){uName.value=a;uDisplay.value=b;uRole.value=c||'user';uStatus.value=d||'active';uPass.value=''}
function clearUserForm(){uName.value='';uDisplay.value='';uRole.value='user';uStatus.value='active';uPass.value=''}
async function saveUser(){try{await api('/v1/admin/users',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:uName.value,display_name:uDisplay.value,role:uRole.value,status:uStatus.value,password:uPass.value})});clearUserForm();loadUsers();toast('用户已保存')}catch(e){toast(e.message)}}
async function deleteUser(name){if(!confirm('确认删除用户 '+name+' ?'))return;try{await api('/v1/admin/users/'+encodeURIComponent(name),{method:'DELETE'});loadUsers()}catch(e){toast(e.message)}}
async function logout(){try{await api('/v1/admin/auth/logout',{method:'POST'});location.href='/admin/login'}catch(e){location.href='/admin/login'}}
loadMe();loadDatasources();loadJobs();loadSettings();loadExports();setInterval(function(){if(!document.getElementById('page-jobs').classList.contains('hidden'))loadJobs();if(currentJobID&&!document.getElementById('page-jobs').classList.contains('hidden'))loadDetail(currentJobID)},10000);
</script>
</body>
</html>`
