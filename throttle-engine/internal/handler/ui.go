package handler

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"github.com/oche/homelab-observability/throttle-engine/internal/inventory"
)

type UIHandler struct {
	webhook   *WebhookHandler
	inventory *inventory.Inventory
}

func NewUIHandler(wh *WebhookHandler, inv *inventory.Inventory) *UIHandler {
	return &UIHandler{webhook: wh, inventory: inv}
}

func (u *UIHandler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

func (u *UIHandler) HandleAPIAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u.webhook.GetHistory())
}

func (u *UIHandler) HandleAPIStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u.webhook.GetStats())
}

func (u *UIHandler) HandleUpdateDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		var req struct {
			IP    string `json:"ip"`
			Name  string `json:"name"`
			Owner string `json:"owner"`
			Type  string `json:"type"`
			MAC   string `json:"mac"`
			Notes string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.IP == "" {
			http.Error(w, "ip required", http.StatusBadRequest)
			return
		}
		dev := inventory.Device{Name: req.Name, Owner: req.Owner, Type: req.Type, MAC: req.MAC, Notes: req.Notes}
		if err := u.inventory.UpdateDevice(req.IP, dev); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated", "ip": req.IP})
		return
	}
	if r.Method == http.MethodDelete {
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "ip required", http.StatusBadRequest)
			return
		}
		if err := u.inventory.DeleteDevice(req.IP); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "ip": req.IP})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

type ScanResult struct {
	IP     string            `json:"ip"`
	MAC    string            `json:"mac"`
	State  string            `json:"state"`
	Known  bool              `json:"known"`
	Device *inventory.Device `json:"device,omitempty"`
}

func (u *UIHandler) HandleNetworkScan(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("nmap", "-sn", "192.168.1.0/24", "-oG", "-").Output()
	if err != nil {
		out2, _ := exec.Command("ip", "neigh", "show", "dev", "eno1").Output()
		results := parseIPNeigh(string(out2), u.inventory)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
		return
	}
	results := parseNmapGrepable(string(out), u.inventory)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func parseNmapGrepable(output string, inv *inventory.Inventory) []ScanResult {
	var results []ScanResult
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "Host:") || !strings.Contains(line, "Up") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		ip := parts[1]
		if strings.HasPrefix(ip, "172.") {
			continue
		}
		sr := ScanResult{IP: ip, State: "up"}
		if dev, ok := inv.Lookup(ip); ok {
			sr.Known = true
			sr.Device = &dev
			sr.MAC = dev.MAC
		}
		results = append(results, sr)
	}
	return results
}

func parseIPNeigh(output string, inv *inventory.Inventory) []ScanResult {
	var results []ScanResult
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ip := fields[0]
		if strings.HasPrefix(ip, "172.") || strings.HasPrefix(ip, "fe80") {
			continue
		}
		mac := ""
		state := ""
		for i, f := range fields {
			if f == "lladdr" && i+1 < len(fields) {
				mac = fields[i+1]
			}
		}
		if len(fields) > 0 {
			state = strings.ToLower(fields[len(fields)-1])
		}
		sr := ScanResult{IP: ip, MAC: mac, State: state}
		if dev, ok := inv.Lookup(ip); ok {
			sr.Known = true
			sr.Device = &dev
		}
		results = append(results, sr)
	}
	return results
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Throttle Engine</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0f1117;color:#e0e0e0}
.hdr{background:#1a1d27;padding:14px 24px;border-bottom:1px solid #2a2d3a;display:flex;align-items:center;justify-content:space-between}
.hdr h1{font-size:18px;font-weight:600;color:#fff}.hdr h1 span{color:#6c5ce7}
.hdr .st{display:flex;align-items:center;gap:8px;font-size:12px;color:#888}
.hdr .dot{width:8px;height:8px;border-radius:50%;background:#00b894;animation:pulse 2s infinite}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.4}}
.ctn{max-width:1400px;margin:0 auto;padding:20px}
.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:10px;margin-bottom:16px}
.sc{background:#1a1d27;border-radius:8px;padding:14px;border:1px solid #2a2d3a}
.sc .lb{font-size:10px;text-transform:uppercase;letter-spacing:1px;color:#555;margin-bottom:4px}
.sc .vl{font-size:24px;font-weight:700;color:#fff}
.sc .vl.f{color:#e74c3c}.sc .vl.r{color:#00b894}.sc .vl.t{color:#6c5ce7}
.sc .sub{font-size:10px;color:#555;margin-top:2px}
.charts{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:16px}
.chart-box{background:#1a1d27;border-radius:8px;border:1px solid #2a2d3a;padding:14px}
.chart-box h3{font-size:12px;color:#888;margin-bottom:10px;text-transform:uppercase;letter-spacing:1px}
canvas{width:100%!important;height:200px!important}
.gauges{display:grid;grid-template-columns:repeat(4,1fr);gap:10px;margin-bottom:16px}
.gauge{background:#1a1d27;border-radius:8px;border:1px solid #2a2d3a;padding:14px;text-align:center}
.gauge .ring{position:relative;width:80px;height:80px;margin:0 auto 8px}
.gauge .ring svg{transform:rotate(-90deg)}
.gauge .ring .val{position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);font-size:16px;font-weight:700;color:#fff}
.gauge .lb{font-size:11px;color:#888}
.sec{background:#1a1d27;border-radius:8px;border:1px solid #2a2d3a;margin-bottom:16px;overflow:hidden}
.tabs{display:flex;border-bottom:1px solid #2a2d3a}
.tab{padding:10px 16px;cursor:pointer;font-size:12px;color:#888;border-bottom:2px solid transparent;user-select:none}
.tab.a{color:#6c5ce7;border-bottom-color:#6c5ce7}.tab:hover{color:#fff}
.tc{display:none}.tc.a{display:block}
table{width:100%;border-collapse:collapse}
th{text-align:left;padding:8px 12px;font-size:10px;text-transform:uppercase;letter-spacing:1px;color:#555;border-bottom:1px solid #2a2d3a;position:sticky;top:0;background:#1a1d27}
td{padding:9px 12px;font-size:12px;border-bottom:1px solid #1f222e}
tr:hover{background:#1f222e}
.bf{background:#e74c3c22;color:#e74c3c;padding:2px 8px;border-radius:4px;font-size:10px;font-weight:600}
.br{background:#00b89422;color:#00b894;padding:2px 8px;border-radius:4px;font-size:10px;font-weight:600}
.bw{background:#f39c1222;color:#f39c12;padding:2px 8px;border-radius:4px;font-size:10px}
.bc{background:#e74c3c22;color:#e74c3c;padding:2px 8px;border-radius:4px;font-size:10px}
.bd{background:#6c5ce722;color:#6c5ce7;padding:2px 8px;border-radius:4px;font-size:10px}
.bu{background:#74b9ff22;color:#74b9ff;padding:2px 8px;border-radius:4px;font-size:10px}
.empty{padding:30px;text-align:center;color:#555;font-size:12px}
.dg{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:10px;padding:12px}
.dc{background:#0f1117;border:1px solid #2a2d3a;border-radius:6px;padding:12px;position:relative}
.dc .ip{font-family:monospace;font-size:12px;color:#6c5ce7;margin-bottom:3px}
.dc .nm{font-size:13px;font-weight:600;color:#fff}
.dc .mt{font-size:10px;color:#666;margin-top:3px}
.dc .mac{font-family:monospace;font-size:10px;color:#444;margin-top:2px}
.dc .acts{position:absolute;top:8px;right:8px;display:flex;gap:4px}
.btn{padding:4px 10px;border:1px solid #2a2d3a;background:#1a1d27;color:#ddd;border-radius:4px;cursor:pointer;font-size:10px}
.btn:hover{background:#2a2d3a;color:#fff}
.btn.p{background:#6c5ce722;border-color:#6c5ce7;color:#6c5ce7}
.btn.p:hover{background:#6c5ce744}
.btn.d{color:#e74c3c;border-color:#e74c3c44}.btn.d:hover{background:#e74c3c22}
.btn.scan{background:#00b89422;border-color:#00b894;color:#00b894;padding:5px 14px;font-size:11px}
.btn.scan:hover{background:#00b89444}
.toolbar{padding:10px 12px;display:flex;gap:8px;align-items:center;border-bottom:1px solid #2a2d3a}
.modal-bg{position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,.6);display:none;z-index:100;justify-content:center;align-items:center}
.modal-bg.show{display:flex}
.modal{background:#1a1d27;border:1px solid #2a2d3a;border-radius:8px;padding:20px;width:400px;max-width:90vw}
.modal h3{margin-bottom:14px;font-size:14px;color:#fff}
.modal label{display:block;font-size:10px;text-transform:uppercase;letter-spacing:1px;color:#666;margin:8px 0 3px}
.modal input,.modal select{width:100%;padding:7px 9px;background:#0f1117;border:1px solid #2a2d3a;border-radius:4px;color:#e0e0e0;font-size:12px}
.modal input:focus,.modal select:focus{outline:none;border-color:#6c5ce7}
.modal .btns{display:flex;gap:6px;margin-top:14px;justify-content:flex-end}
.topo{padding:16px;min-height:300px}
.topo-container{display:flex;flex-wrap:wrap;gap:20px;justify-content:center;align-items:flex-start}
.topo-center{text-align:center;margin:0 30px}
.topo-router,.topo-server{background:#1a1d27;border:2px solid #6c5ce7;border-radius:12px;padding:16px 24px;text-align:center;position:relative}
.topo-router{border-color:#f39c12}
.topo-router .icon,.topo-server .icon{font-size:28px;margin-bottom:4px}
.topo-router .name,.topo-server .name{font-size:12px;font-weight:600;color:#fff}
.topo-router .ip,.topo-server .ip{font-size:10px;color:#888;font-family:monospace}
.topo-line{width:2px;height:30px;background:#2a2d3a;margin:0 auto}
.topo-devices{display:flex;flex-wrap:wrap;gap:10px;justify-content:center;max-width:800px}
.topo-dev{background:#0f1117;border:1px solid #2a2d3a;border-radius:8px;padding:10px 14px;text-align:center;min-width:100px}
.topo-dev.online{border-color:#00b89466}
.topo-dev.offline{border-color:#e74c3c44;opacity:.5}
.topo-dev .icon{font-size:20px;margin-bottom:2px}
.topo-dev .name{font-size:11px;font-weight:600;color:#ddd}
.topo-dev .ip{font-size:9px;color:#666;font-family:monospace}
.topo-dev .owner{font-size:9px;color:#555}
@media(max-width:900px){.charts,.gauges{grid-template-columns:1fr}}
</style>
</head>
<body>
<div class="hdr">
  <h1><span>&#9889;</span> Throttle Engine</h1>
  <div class="st"><div class="dot"></div>Live &middot; 10s refresh</div>
</div>
<div class="ctn">
  <div class="stats">
    <div class="sc"><div class="lb">Alerts</div><div class="vl t" id="s-total">0</div></div>
    <div class="sc"><div class="lb">Firing</div><div class="vl f" id="s-firing">0</div></div>
    <div class="sc"><div class="lb">Resolved</div><div class="vl r" id="s-resolved">0</div></div>
    <div class="sc"><div class="lb">Devices</div><div class="vl" id="s-devices">0</div></div>
    <div class="sc"><div class="lb">Online Now</div><div class="vl r" id="s-online">-</div></div>
    <div class="sc"><div class="lb">Uptime</div><div class="vl" id="s-uptime" style="font-size:18px">-</div></div>
  </div>

  <div class="gauges">
    <div class="gauge"><div class="ring"><svg viewBox="0 0 36 36"><path d="M18 2.0845a15.9155 15.9155 0 0 1 0 31.831a15.9155 15.9155 0 0 1 0-31.831" fill="none" stroke="#2a2d3a" stroke-width="3"/><path id="g-cpu-path" d="M18 2.0845a15.9155 15.9155 0 0 1 0 31.831a15.9155 15.9155 0 0 1 0-31.831" fill="none" stroke="#6c5ce7" stroke-width="3" stroke-dasharray="0, 100" stroke-linecap="round"/></svg><div class="val" id="g-cpu">-</div></div><div class="lb">CPU</div></div>
    <div class="gauge"><div class="ring"><svg viewBox="0 0 36 36"><path d="M18 2.0845a15.9155 15.9155 0 0 1 0 31.831a15.9155 15.9155 0 0 1 0-31.831" fill="none" stroke="#2a2d3a" stroke-width="3"/><path id="g-mem-path" d="M18 2.0845a15.9155 15.9155 0 0 1 0 31.831a15.9155 15.9155 0 0 1 0-31.831" fill="none" stroke="#00b894" stroke-width="3" stroke-dasharray="0, 100" stroke-linecap="round"/></svg><div class="val" id="g-mem">-</div></div><div class="lb">Memory</div></div>
    <div class="gauge"><div class="ring"><svg viewBox="0 0 36 36"><path d="M18 2.0845a15.9155 15.9155 0 0 1 0 31.831a15.9155 15.9155 0 0 1 0-31.831" fill="none" stroke="#2a2d3a" stroke-width="3"/><path id="g-disk-path" d="M18 2.0845a15.9155 15.9155 0 0 1 0 31.831a15.9155 15.9155 0 0 1 0-31.831" fill="none" stroke="#fdcb6e" stroke-width="3" stroke-dasharray="0, 100" stroke-linecap="round"/></svg><div class="val" id="g-disk">-</div></div><div class="lb">Disk Free</div></div>
    <div class="gauge"><div class="ring"><svg viewBox="0 0 36 36"><path d="M18 2.0845a15.9155 15.9155 0 0 1 0 31.831a15.9155 15.9155 0 0 1 0-31.831" fill="none" stroke="#2a2d3a" stroke-width="3"/><path id="g-conn-path" d="M18 2.0845a15.9155 15.9155 0 0 1 0 31.831a15.9155 15.9155 0 0 1 0-31.831" fill="none" stroke="#74b9ff" stroke-width="3" stroke-dasharray="0, 100" stroke-linecap="round"/></svg><div class="val" id="g-conn">-</div></div><div class="lb">TCP Conn</div></div>
  </div>

  <div class="charts">
    <div class="chart-box"><h3>Bandwidth — Receive</h3><canvas id="chart-rx"></canvas></div>
    <div class="chart-box"><h3>Bandwidth — Transmit</h3><canvas id="chart-tx"></canvas></div>
  </div>

  <div class="sec">
    <div class="tabs">
      <div class="tab a" onclick="switchTab(this,'topo')">Network Map</div>
      <div class="tab" onclick="switchTab(this,'alerts')">Alert History</div>
      <div class="tab" onclick="switchTab(this,'devices')">Devices</div>
      <div class="tab" onclick="switchTab(this,'scan')">Scan</div>
    </div>
    <div class="tc a" id="tab-topo">
      <div class="topo" id="topo-area"><div class="empty">Loading network map...</div></div>
    </div>
    <div class="tc" id="tab-alerts">
      <div style="max-height:400px;overflow-y:auto">
      <table><thead><tr><th>Time</th><th>Alert</th><th>Status</th><th>Severity</th><th>Device</th><th>Owner</th><th>Summary</th></tr></thead>
      <tbody id="alert-body"><tr><td colspan="7" class="empty">No alerts yet.</td></tr></tbody></table>
      </div>
    </div>
    <div class="tc" id="tab-devices">
      <div class="toolbar"><button class="btn p" onclick="openModal()">+ Add Device</button><span style="font-size:10px;color:#555" id="dev-count"></span></div>
      <div class="dg" id="dev-grid"><div class="empty">Loading...</div></div>
    </div>
    <div class="tc" id="tab-scan">
      <div class="toolbar"><button class="btn scan" onclick="runScan()" id="scan-btn">Scan Network</button><span style="font-size:10px;color:#555" id="scan-status"></span></div>
      <div style="max-height:400px;overflow-y:auto">
      <table><thead><tr><th>IP</th><th>MAC</th><th>Status</th><th>Known</th><th>Name</th><th>Action</th></tr></thead>
      <tbody id="scan-body"><tr><td colspan="6" class="empty">Click Scan Network to discover devices.</td></tr></tbody></table>
      </div>
    </div>
  </div>
</div>

<div class="modal-bg" id="modal-bg" onclick="if(event.target===this)closeModal()">
<div class="modal">
  <h3 id="modal-title">Add Device</h3>
  <label>IP Address</label><input id="m-ip" placeholder="192.168.1.x">
  <label>Name</label><input id="m-name" placeholder="My-Laptop">
  <label>Owner</label><input id="m-owner" placeholder="Oche">
  <label>Type</label>
  <select id="m-type"><option value="laptop">Laptop</option><option value="phone">Phone</option><option value="desktop">Desktop</option><option value="server">Server</option><option value="smart_tv">Smart TV</option><option value="iot">IoT</option><option value="game_console">Game Console</option><option value="router">Router</option><option value="printer">Printer</option><option value="camera">Camera</option><option value="guest">Guest</option><option value="unknown">Unknown</option></select>
  <label>MAC Address</label><input id="m-mac" placeholder="aa:bb:cc:dd:ee:ff">
  <label>Notes</label><input id="m-notes" placeholder="Optional notes">
  <div class="btns"><button class="btn" onclick="closeModal()">Cancel</button><button class="btn p" onclick="saveDevice()">Save</button></div>
</div>
</div>

<script>
let devices={},onlineIPs=new Set();
const typeIcons={router:'\u{1F4E1}',server:'\u{1F5A5}',laptop:'\u{1F4BB}',phone:'\u{1F4F1}',desktop:'\u{1F5A5}',smart_tv:'\u{1F4FA}',iot:'\u{2699}',game_console:'\u{1F3AE}',printer:'\u{1F5A8}',camera:'\u{1F4F7}',guest:'\u{1F464}',unknown:'\u{2753}'};

function switchTab(el,name){document.querySelectorAll('.tab').forEach(t=>t.classList.remove('a'));document.querySelectorAll('.tc').forEach(t=>t.classList.remove('a'));el.classList.add('a');document.getElementById('tab-'+name).classList.add('a')}
function ts(d){const s=Math.floor((Date.now()-new Date(d))/1000);if(s<60)return s+'s';if(s<3600)return Math.floor(s/60)+'m';if(s<86400)return Math.floor(s/3600)+'h';return Math.floor(s/86400)+'d'}
function ut(d){const s=Math.floor((Date.now()-new Date(d))/1000);const days=Math.floor(s/86400),h=Math.floor((s%86400)/3600),m=Math.floor((s%3600)/60);if(days>0)return days+'d '+h+'h';return h+'h '+m+'m'}
function fmtBytes(b){if(b<1024)return b.toFixed(0)+' B/s';if(b<1048576)return(b/1024).toFixed(1)+' KB/s';return(b/1048576).toFixed(2)+' MB/s'}
function sb(s){return s==='firing'?'<span class="bf">FIRING</span>':'<span class="br">RESOLVED</span>'}
function svb(s){if(!s)return'-';return s==='critical'?'<span class="bc">'+s+'</span>':'<span class="bw">'+s+'</span>'}
function setGauge(id,pct,label){const p=document.getElementById(id+'-path');const v=document.getElementById(id);if(!p)return;const val=Math.min(100,Math.max(0,pct));p.setAttribute('stroke-dasharray',val+', 100');v.textContent=label||Math.round(val)+'%';const colors=[[85,'#e74c3c'],[60,'#f39c12'],[0,'#00b894']];if(id==='g-disk'){for(const[th,c]of[[30,'#e74c3c'],[50,'#f39c12'],[0,'#00b894']])if(val<=th||th===0){p.setAttribute('stroke',c);break}}else{for(const[th,c]of colors)if(val>=th||th===0){p.setAttribute('stroke',c);break}}}

function openModal(ip,dev){
  document.getElementById('modal-bg').classList.add('show');
  document.getElementById('modal-title').textContent=ip?'Edit Device':'Add Device';
  document.getElementById('m-ip').value=ip||'';document.getElementById('m-ip').readOnly=!!ip;
  if(dev){document.getElementById('m-name').value=dev.name||'';document.getElementById('m-owner').value=dev.owner||'';document.getElementById('m-type').value=dev.type||'unknown';document.getElementById('m-mac').value=dev.mac||'';document.getElementById('m-notes').value=dev.notes||''}
  else{['m-name','m-owner','m-mac','m-notes'].forEach(id=>document.getElementById(id).value='');document.getElementById('m-type').value='unknown'}
}
function closeModal(){document.getElementById('modal-bg').classList.remove('show')}

async function saveDevice(){
  const body={ip:document.getElementById('m-ip').value,name:document.getElementById('m-name').value,owner:document.getElementById('m-owner').value,type:document.getElementById('m-type').value,mac:document.getElementById('m-mac').value,notes:document.getElementById('m-notes').value};
  if(!body.ip||!body.name){alert('IP and Name required');return}
  await fetch('/api/v1/devices/update',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  closeModal();refresh()
}
async function deleteDevice(ip){if(!confirm('Remove '+ip+'?'))return;await fetch('/api/v1/devices/update',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({ip})});refresh()}

async function runScan(){
  const btn=document.getElementById('scan-btn');btn.textContent='Scanning...';btn.disabled=true;
  document.getElementById('scan-status').textContent='nmap -sn 192.168.1.0/24...';
  try{
    const r=await fetch('/api/v1/scan');const hosts=await r.json();
    onlineIPs=new Set(hosts.map(h=>h.ip));
    document.getElementById('s-online').textContent=hosts.length;
    const tb=document.getElementById('scan-body');
    if(!hosts||!hosts.length){tb.innerHTML='<tr><td colspan="6" class="empty">No hosts found.</td></tr>';return}
    tb.innerHTML=hosts.map(h=>{
      const k=h.known,name=h.device?h.device.name:'-';
      const act=k?'<button class="btn" onclick=\'openModal("'+h.ip+'",devices["'+h.ip+'"])\'>Edit</button>':'<button class="btn p" onclick=\'openModal("'+h.ip+'",{mac:"'+(h.mac||'')+'"})\'>+ Add</button>';
      return '<tr><td style="font-family:monospace">'+h.ip+'</td><td style="font-family:monospace;font-size:10px;color:#555">'+(h.mac||'-')+'</td><td><span class="bu">'+h.state+'</span></td><td>'+(k?'<span class="br">Known</span>':'<span class="bw">Unknown</span>')+'</td><td>'+name+'</td><td>'+act+'</td></tr>'
    }).join('');
    document.getElementById('scan-status').textContent=hosts.length+' hosts';
    renderTopo();
  }catch(e){document.getElementById('scan-status').textContent='Failed: '+e}
  finally{btn.textContent='Scan Network';btn.disabled=false}
}

function renderTopo(){
  const area=document.getElementById('topo-area');
  const ips=Object.keys(devices).sort();
  const router=devices['192.168.1.1'];
  const server=devices['192.168.1.200'];
  let html='<div class="topo-container"><div class="topo-center">';
  if(router)html+='<div class="topo-router"><div class="icon">'+typeIcons.router+'</div><div class="name">'+router.name+'</div><div class="ip">192.168.1.1</div></div>';
  html+='<div class="topo-line"></div>';
  if(server)html+='<div class="topo-server"><div class="icon">'+typeIcons.server+'</div><div class="name">'+server.name+'</div><div class="ip">192.168.1.200</div></div>';
  html+='<div class="topo-line"></div></div></div>';
  html+='<div class="topo-devices">';
  ips.filter(ip=>ip!=='192.168.1.1'&&ip!=='192.168.1.200').forEach(ip=>{
    const d=devices[ip];const online=onlineIPs.has(ip);
    const icon=typeIcons[d.type]||typeIcons.unknown;
    html+='<div class="topo-dev '+(online?'online':'offline')+'"><div class="icon">'+icon+'</div><div class="name">'+d.name+'</div><div class="ip">'+ip+'</div><div class="owner">'+d.owner+(online?' \u2022 online':'')+'</div></div>';
  });
  html+='</div>';
  area.innerHTML=html;
}

// Simple canvas chart
function drawChart(canvasId,dataPoints,color){
  const canvas=document.getElementById(canvasId);if(!canvas)return;
  const ctx=canvas.getContext('2d');
  const dpr=window.devicePixelRatio||1;
  const rect=canvas.getBoundingClientRect();
  canvas.width=rect.width*dpr;canvas.height=rect.height*dpr;
  ctx.scale(dpr,dpr);
  const w=rect.width,h=rect.height;
  ctx.clearRect(0,0,w,h);
  if(!dataPoints||!dataPoints.length)return;
  const max=Math.max(...dataPoints.map(p=>p[1]),1);
  const xStep=w/(dataPoints.length-1||1);
  // Grid lines
  ctx.strokeStyle='#1f222e';ctx.lineWidth=1;
  for(let i=0;i<4;i++){const y=h*(i/3);ctx.beginPath();ctx.moveTo(0,y);ctx.lineTo(w,y);ctx.stroke()}
  // Line
  ctx.strokeStyle=color;ctx.lineWidth=2;ctx.lineJoin='round';
  ctx.beginPath();
  dataPoints.forEach((p,i)=>{const x=i*xStep,y=h-(p[1]/max)*h*0.9;i===0?ctx.moveTo(x,y):ctx.lineTo(x,y)});
  ctx.stroke();
  // Fill
  const last=dataPoints.length-1;
  ctx.lineTo(last*xStep,h);ctx.lineTo(0,h);ctx.closePath();
  ctx.fillStyle=color+'18';ctx.fill();
  // Labels
  ctx.fillStyle='#555';ctx.font='10px sans-serif';
  ctx.fillText(fmtBytes(max),4,12);
  ctx.fillText('0',4,h-4);
  ctx.fillText('1h ago',4,h-16);
  ctx.fillText('now',w-30,h-16);
}

async function loadBandwidth(){
  try{
    const r=await fetch('/api/v1/metrics/bandwidth');const data=await r.json();
    if(data.rx&&data.rx.data&&data.rx.data.result){
      const series=data.rx.data.result;
      if(series.length>0)drawChart('chart-rx',series[0].values.map(v=>[v[0],parseFloat(v[1])]),'#6c5ce7');
    }
    if(data.tx&&data.tx.data&&data.tx.data.result){
      const series=data.tx.data.result;
      if(series.length>0)drawChart('chart-tx',series[0].values.map(v=>[v[0],parseFloat(v[1])]),'#00b894');
    }
  }catch(e){console.error('bandwidth fetch failed',e)}
}

async function loadSystem(){
  try{
    const r=await fetch('/api/v1/metrics/system');const data=await r.json();
    if(data.cpu&&data.cpu.data&&data.cpu.data.result&&data.cpu.data.result[0])
      setGauge('g-cpu',parseFloat(data.cpu.data.result[0].value[1]));
    if(data.memory&&data.memory.data&&data.memory.data.result&&data.memory.data.result[0])
      setGauge('g-mem',parseFloat(data.memory.data.result[0].value[1]));
    if(data.disk&&data.disk.data&&data.disk.data.result&&data.disk.data.result[0])
      setGauge('g-disk',parseFloat(data.disk.data.result[0].value[1]));
    if(data.connections&&data.connections.data&&data.connections.data.result&&data.connections.data.result[0]){
      const c=parseInt(data.connections.data.result[0].value[1]);
      setGauge('g-conn',Math.min(c,100),c.toString());}
  }catch(e){console.error('system fetch failed',e)}
}

async function refresh(){
  try{
    const[sr,ar,dr]=await Promise.all([fetch('/api/v1/stats'),fetch('/api/v1/alerts'),fetch('/api/v1/devices')]);
    const stats=await sr.json();const alerts=await ar.json();devices=await dr.json();
    document.getElementById('s-total').textContent=stats.total_received;
    document.getElementById('s-firing').textContent=stats.total_firing;
    document.getElementById('s-resolved').textContent=stats.total_resolved;
    document.getElementById('s-uptime').textContent=ut(stats.started_at);
    const ips=Object.keys(devices);
    document.getElementById('s-devices').textContent=ips.length;
    document.getElementById('dev-count').textContent=ips.length+' devices';

    const tb=document.getElementById('alert-body');
    if(alerts&&alerts.length>0){tb.innerHTML=alerts.map(a=>'<tr><td style="white-space:nowrap">'+ts(a.timestamp)+'</td><td>'+(a.alert_name||'-')+'</td><td>'+sb(a.status)+'</td><td>'+svb(a.severity)+'</td><td>'+(a.device_name||a.instance||'-')+'</td><td>'+(a.owner||'-')+'</td><td style="max-width:250px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+(a.summary||'-')+'</td></tr>').join('')}

    const grid=document.getElementById('dev-grid');
    if(ips.length>0){grid.innerHTML=ips.sort().map(ip=>{const d=devices[ip];const icon=typeIcons[d.type]||typeIcons.unknown;const online=onlineIPs.has(ip);
      return '<div class="dc"><div class="acts"><button class="btn" onclick=\'openModal("'+ip+'",devices["'+ip+'"])\'>Edit</button><button class="btn d" onclick="deleteDevice(\''+ip+'\')">Del</button></div><div class="ip">'+icon+' '+ip+(online?' <span class="br" style="font-size:9px">online</span>':'')+'</div><div class="nm">'+d.name+'</div><div class="mt">'+d.owner+' &middot; <span class="bd">'+d.type+'</span></div>'+(d.mac?'<div class="mac">'+d.mac+'</div>':'')+'</div>'}).join('')}

    renderTopo();
    loadBandwidth();
    loadSystem();
  }catch(e){console.error('refresh failed',e)}
}

refresh();
// Initial scan to populate online status
fetch('/api/v1/scan').then(r=>r.json()).then(hosts=>{onlineIPs=new Set(hosts.map(h=>h.ip));document.getElementById('s-online').textContent=hosts.length;renderTopo()}).catch(()=>{});
setInterval(refresh,10000);
</script>
</body>
</html>`
