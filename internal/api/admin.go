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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(adminHTML))
}

func (s *Server) adminJobs(w http.ResponseWriter, r *http.Request) {
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

const adminHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>OpenClaw NL2SQL 管理后台</title>
<style>
:root{--bg:#f6f8fb;--card:#fff;--line:#e5e7eb;--text:#111827;--muted:#6b7280;--primary:#2563eb;--ok:#16a34a;--bad:#dc2626;--warn:#d97706;--code:#0f172a}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,"Noto Sans SC",sans-serif}.wrap{max-width:1380px;margin:0 auto;padding:20px}.top{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:16px}.brand h1{margin:0;font-size:24px}.brand p{margin:6px 0 0;color:var(--muted)}.tabs{display:flex;gap:8px;flex-wrap:wrap}.tab{border:1px solid var(--line);background:#fff;border-radius:999px;padding:9px 14px;cursor:pointer}.tab.active{background:var(--primary);border-color:var(--primary);color:#fff}.grid{display:grid;grid-template-columns:1fr 420px;gap:16px}.card{background:var(--card);border:1px solid var(--line);border-radius:16px;box-shadow:0 8px 26px rgba(15,23,42,.05);padding:16px}.toolbar{display:flex;gap:10px;flex-wrap:wrap;margin-bottom:12px}.toolbar input,.toolbar select,.toolbar button,textarea{border:1px solid var(--line);border-radius:10px;padding:9px 11px;background:#fff;font-size:14px}.toolbar input{min-width:220px}button{background:var(--primary);color:#fff;border:0;border-radius:10px;padding:9px 13px;cursor:pointer}button.secondary{background:#475569}button.danger{background:var(--bad)}button.ghost{background:#eef2ff;color:#1d4ed8}.hint{color:var(--muted);font-size:13px}table{width:100%;border-collapse:collapse;font-size:13px}th,td{border-bottom:1px solid var(--line);text-align:left;padding:10px 8px;vertical-align:top}th{color:#374151;background:#f9fafb;position:sticky;top:0}.status{display:inline-block;border-radius:999px;padding:3px 8px;font-size:12px;background:#e5e7eb}.status.sent,.status.sent_cached{background:#dcfce7;color:#166534}.status.failed,.status.rejected{background:#fee2e2;color:#991b1b}.status.running,.status.validating,.status.queued{background:#fef3c7;color:#92400e}.mono,pre{font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,"Liberation Mono","Courier New",monospace}.ellipsis{max-width:280px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.detail h3{margin:0 0 10px}.section{margin-top:14px}.section label{display:block;font-size:13px;color:#374151;margin:0 0 6px}pre{white-space:pre-wrap;word-break:break-word;margin:0;background:var(--code);color:#e5e7eb;border-radius:12px;padding:12px;max-height:360px;overflow:auto}.events{max-height:260px;overflow:auto}.event{border-left:3px solid #cbd5e1;padding:6px 8px;margin:8px 0;background:#f8fafc}.manual textarea{width:100%;min-height:260px}.manual .row{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:10px}.manual input,.manual select{width:100%;border:1px solid var(--line);border-radius:10px;padding:9px 11px}.toast{position:fixed;right:18px;bottom:18px;background:#111827;color:white;border-radius:12px;padding:12px 14px;box-shadow:0 8px 30px rgba(0,0,0,.2);display:none;max-width:520px}.smallbtn{padding:6px 9px;font-size:12px}#manualPage{display:none}.resultTable{overflow:auto;max-height:360px}.resultTable table{font-size:12px}.pill{display:inline-block;background:#eef2ff;color:#3730a3;border-radius:999px;padding:3px 8px;font-size:12px;margin-right:5px}@media(max-width:1050px){.grid{grid-template-columns:1fr}.manual .row{grid-template-columns:1fr}.ellipsis{max-width:160px}}</style>
</head>
<body>
<div class="wrap">
  <div class="top">
    <div class="brand"><h1>OpenClaw NL2SQL 管理后台</h1><p>查看任务事件、SQL、结果；支持重跑任务和手动 SQL 查询并私聊发送。</p></div>
    <div class="tabs"><button class="tab active" id="tabJobs" onclick="showPage('jobs')">任务管理</button><button class="tab" id="tabManual" onclick="showPage('manual')">手动查询</button></div>
  </div>

  <div id="jobsPage" class="grid">
    <div class="card">
      <div class="toolbar">
        <input id="q" placeholder="搜索任务ID / 用户 / SQL / 错误 / 结果" onkeydown="if(event.key==='Enter')loadJobs()">
        <select id="status"><option value="">全部状态</option><option value="queued">queued</option><option value="validating">validating</option><option value="running">running</option><option value="sent">sent</option><option value="sent_cached">sent_cached</option><option value="failed">failed</option><option value="rejected">rejected</option></select>
        <select id="limit"><option>20</option><option selected>50</option><option>100</option><option>200</option></select>
        <button onclick="loadJobs()">刷新</button>
      </div>
      <div class="hint" id="jobCount">加载中...</div>
      <div style="overflow:auto;max-height:72vh;margin-top:10px"><table><thead><tr><th>时间</th><th>状态</th><th>任务</th><th>用户</th><th>问题/SQL</th><th>结果</th><th>操作</th></tr></thead><tbody id="jobsBody"></tbody></table></div>
    </div>
    <div class="card detail">
      <h3>任务详情</h3>
      <div id="detailEmpty" class="hint">点击左侧“详情”查看 SQL、结果和事件。</div>
      <div id="detailBox" style="display:none">
        <div><span id="dStatus" class="status"></span> <span class="pill" id="dID"></span> <span class="pill" id="dDS"></span></div>
        <div class="section"><label>问题</label><div id="dQuestion" class="hint"></div></div>
        <div class="section"><label>执行 SQL</label><pre id="dSQL"></pre></div>
        <div class="section"><label>查询结果</label><pre id="dResult"></pre></div>
        <div class="section"><label>结果预览</label><div id="dRows" class="resultTable"></div></div>
        <div class="section"><label>事件记录</label><div id="dEvents" class="events"></div></div>
        <div class="section"><button onclick="rerunCurrent()">重新执行并发送给原用户</button> <button class="secondary" onclick="copySQL()">复制 SQL</button></div>
      </div>
    </div>
  </div>

  <div id="manualPage" class="card manual">
    <div class="row"><div><label>数据源</label><select id="mDatasource"><option value="">自动路由</option></select></div><div><label>发送到用户/Chat ID</label><input id="mChat" placeholder="Telegram 用户ID或Chat ID；为空则只在后台查看"></div></div>
    <div class="row"><div><label>问题描述</label><input id="mQuestion" placeholder="例如：手动排查昨日美国VPBET充值提现"></div><div><label>Request ID（可选）</label><input id="mRequest" placeholder="为空自动生成"></div></div>
    <label>SQL</label><textarea id="mSQL" class="mono" placeholder="SELECT ..."></textarea>
    <div class="toolbar" style="margin-top:12px"><button onclick="executeManual()">执行查询</button><button class="secondary" onclick="document.getElementById('mSQL').value=''">清空</button></div>
    <div class="hint" id="mResult"></div>
  </div>
</div>
<div class="toast" id="toast"></div>
<script>
var currentJobID='';
function esc(s){return (s==null?'':String(s)).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
function fmtTime(s){if(!s)return '';try{return new Date(s).toLocaleString()}catch(e){return s}}
function showToast(s){var t=document.getElementById('toast');t.textContent=s;t.style.display='block';setTimeout(function(){t.style.display='none'},3200)}
function showPage(p){document.getElementById('jobsPage').style.display=p==='jobs'?'grid':'none';document.getElementById('manualPage').style.display=p==='manual'?'block':'none';document.getElementById('tabJobs').className='tab '+(p==='jobs'?'active':'');document.getElementById('tabManual').className='tab '+(p==='manual'?'active':'')}
async function api(path,opt){var r=await fetch(path,opt||{});var j=await r.json().catch(function(){return {error:'invalid json'}});if(!r.ok)throw new Error(j.error||r.statusText);return j}
async function loadJobs(){try{var qs=new URLSearchParams({q:document.getElementById('q').value,status:document.getElementById('status').value,limit:document.getElementById('limit').value});var data=await api('/v1/admin/jobs?'+qs.toString());document.getElementById('jobCount').textContent='共 '+data.total+' 条，当前显示 '+(data.jobs||[]).length+' 条';var html='';(data.jobs||[]).forEach(function(j){var sql=j.rewritten_sql||j.sql||'';var title=j.question||sql;html+='<tr><td>'+esc(fmtTime(j.created_at))+'</td><td><span class="status '+esc(j.status)+'">'+esc(j.status)+'</span></td><td class="mono">'+esc(j.id)+'</td><td>'+esc(j.chat_id||j.user_id||'')+'</td><td><div class="ellipsis" title="'+esc(title)+'">'+esc(title)+'</div>'+(j.error?'<div class="hint" style="color:#dc2626">'+esc(j.error)+'</div>':'')+'</td><td>'+esc(j.result_row_count||0)+' 行<br><span class="hint">'+esc(j.result_duration||0)+' ms</span></td><td><button class="smallbtn ghost" onclick="loadDetail(\''+esc(j.id)+'\')">详情</button> <button class="smallbtn" onclick="rerun(\''+esc(j.id)+'\')">重跑</button></td></tr>'});document.getElementById('jobsBody').innerHTML=html||'<tr><td colspan="7" class="hint">暂无记录</td></tr>'}catch(e){showToast(e.message)}}
async function loadDetail(id){try{var j=await api('/v1/admin/jobs/'+encodeURIComponent(id));currentJobID=j.id;document.getElementById('detailEmpty').style.display='none';document.getElementById('detailBox').style.display='block';document.getElementById('dStatus').className='status '+esc(j.status);document.getElementById('dStatus').textContent=j.status;document.getElementById('dID').textContent=j.id;document.getElementById('dDS').textContent=j.data_source_id||j.request?.data_source_id||'自动路由';document.getElementById('dQuestion').textContent=j.request?.question||'';document.getElementById('dSQL').textContent=j.rewritten_sql||j.request?.sql||'';document.getElementById('dResult').textContent=j.result_text||j.error||'暂无结果';renderRows(j);renderEvents(j.events||[])}catch(e){showToast(e.message)}}
function renderRows(j){var cols=j.result_columns||[], rows=j.result_rows||[];if(!cols.length){document.getElementById('dRows').innerHTML='<div class="hint">暂无行预览</div>';return}var html='<table><thead><tr>';cols.forEach(function(c){html+='<th>'+esc(c)+'</th>'});html+='</tr></thead><tbody>';rows.forEach(function(r){html+='<tr>';cols.forEach(function(c,i){html+='<td>'+esc(r[i]||'')+'</td>'});html+='</tr>'});html+='</tbody></table>';if((j.result_row_count||0)>rows.length)html+='<div class="hint">仅展示前 '+rows.length+' 行 / 总 '+j.result_row_count+' 行</div>';document.getElementById('dRows').innerHTML=html}
function renderEvents(events){var html='';events.forEach(function(e){html+='<div class="event"><b>'+esc(e.type)+'</b> <span class="hint">'+esc(fmtTime(e.at))+'</span><br>'+esc(e.message||'')+'</div>'});document.getElementById('dEvents').innerHTML=html||'<div class="hint">暂无事件</div>'}
async function rerun(id){try{var data=await api('/v1/admin/jobs/'+encodeURIComponent(id)+'/rerun',{method:'POST'});showToast('已提交重跑任务：'+data.job_id);loadJobs();setTimeout(function(){loadDetail(data.job_id)},800)}catch(e){showToast(e.message)}}
function rerunCurrent(){if(currentJobID)rerun(currentJobID)}
function copySQL(){var sql=document.getElementById('dSQL').textContent;navigator.clipboard&&navigator.clipboard.writeText(sql);showToast('SQL 已复制')}
async function loadDatasources(){try{var data=await api('/v1/datasources');var sel=document.getElementById('mDatasource');(data.datasources||[]).forEach(function(d){var o=document.createElement('option');o.value=d.id;o.textContent=d.id+(d.default?'（默认）':'')+' - '+(d.description||'');sel.appendChild(o)})}catch(e){}}
async function executeManual(){try{var payload={data_source_id:document.getElementById('mDatasource').value,chat_id:document.getElementById('mChat').value,user_id:document.getElementById('mChat').value,question:document.getElementById('mQuestion').value,request_id:document.getElementById('mRequest').value,sql:document.getElementById('mSQL').value};var data=await api('/v1/admin/sql/execute',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});document.getElementById('mResult').textContent='已提交任务：'+data.job_id+'；执行完成后会在任务管理中显示，若填写了用户ID也会私聊发送。';showPage('jobs');loadJobs();setTimeout(function(){loadDetail(data.job_id)},800)}catch(e){showToast(e.message)}}
loadDatasources();loadJobs();setInterval(function(){if(document.getElementById('jobsPage').style.display!=='none')loadJobs();if(currentJobID)loadDetail(currentJobID)},10000);
</script>
</body>
</html>`
