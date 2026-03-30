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

// HandleUpdateDevice handles PUT /api/v1/devices/:ip
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

// ScanResult represents a discovered device from network scan.
type ScanResult struct {
	IP     string `json:"ip"`
	MAC    string `json:"mac"`
	State  string `json:"state"`
	Known  bool   `json:"known"`
	Device *inventory.Device `json:"device,omitempty"`
}

// HandleNetworkScan runs nmap and returns discovered hosts.
func (u *UIHandler) HandleNetworkScan(w http.ResponseWriter, r *http.Request) {
	// Run nmap ping scan on the local subnet
	out, err := exec.Command("nmap", "-sn", "192.168.1.0/24", "-oG", "-").Output()
	if err != nil {
		// Fallback: parse ip neigh
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
		// Skip Docker bridge IPs
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
.hdr{background:#1a1d27;padding:16px 24px;border-bottom:1px solid #2a2d3a;display:flex;align-items:center;justify-content:space-between}
.hdr h1{font-size:18px;font-weight:600;color:#fff}
.hdr h1 span{color:#6c5ce7}
.hdr .st{display:flex;align-items:center;gap:8px;font-size:12px;color:#888}
.hdr .dot{width:8px;height:8px;border-radius:50%;background:#00b894;animation:pulse 2s infinite}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.4}}
.ctn{max-width:1280px;margin:0 auto;padding:20px}
.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;margin-bottom:20px}
.sc{background:#1a1d27;border-radius:8px;padding:16px;border:1px solid #2a2d3a}
.sc .lb{font-size:11px;text-transform:uppercase;letter-spacing:1px;color:#666;margin-bottom:6px}
.sc .vl{font-size:26px;font-weight:700;color:#fff}
.sc .vl.f{color:#e74c3c}.sc .vl.r{color:#00b894}.sc .vl.t{color:#6c5ce7}
.sec{background:#1a1d27;border-radius:8px;border:1px solid #2a2d3a;margin-bottom:20px;overflow:hidden}
.tabs{display:flex;border-bottom:1px solid #2a2d3a}
.tab{padding:11px 18px;cursor:pointer;font-size:13px;color:#888;border-bottom:2px solid transparent;user-select:none}
.tab.a{color:#6c5ce7;border-bottom-color:#6c5ce7}
.tab:hover{color:#fff}
.tc{display:none}.tc.a{display:block}
table{width:100%;border-collapse:collapse}
th{text-align:left;padding:9px 14px;font-size:10px;text-transform:uppercase;letter-spacing:1px;color:#555;border-bottom:1px solid #2a2d3a;position:sticky;top:0;background:#1a1d27}
td{padding:10px 14px;font-size:13px;border-bottom:1px solid #1f222e}
tr:hover{background:#1f222e}
.bf{background:#e74c3c22;color:#e74c3c;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600}
.br{background:#00b89422;color:#00b894;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600}
.bw{background:#f39c1222;color:#f39c12;padding:2px 8px;border-radius:4px;font-size:11px}
.bc{background:#e74c3c22;color:#e74c3c;padding:2px 8px;border-radius:4px;font-size:11px}
.bd{background:#6c5ce722;color:#6c5ce7;padding:2px 8px;border-radius:4px;font-size:11px}
.bu{background:#74b9ff22;color:#74b9ff;padding:2px 8px;border-radius:4px;font-size:11px}
.empty{padding:40px;text-align:center;color:#555;font-size:13px}
.dg{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:12px;padding:14px}
.dc{background:#0f1117;border:1px solid #2a2d3a;border-radius:6px;padding:14px;position:relative}
.dc .ip{font-family:monospace;font-size:13px;color:#6c5ce7;margin-bottom:4px}
.dc .nm{font-size:14px;font-weight:600;color:#fff}
.dc .mt{font-size:11px;color:#666;margin-top:4px}
.dc .mac{font-family:monospace;font-size:11px;color:#555;margin-top:2px}
.dc .acts{position:absolute;top:10px;right:10px;display:flex;gap:6px}
.btn{padding:5px 12px;border:1px solid #2a2d3a;background:#1a1d27;color:#ddd;border-radius:4px;cursor:pointer;font-size:11px}
.btn:hover{background:#2a2d3a;color:#fff}
.btn.p{background:#6c5ce722;border-color:#6c5ce7;color:#6c5ce7}
.btn.p:hover{background:#6c5ce744}
.btn.d{color:#e74c3c;border-color:#e74c3c44}
.btn.d:hover{background:#e74c3c22}
.btn.scan{background:#00b89422;border-color:#00b894;color:#00b894;padding:6px 16px;font-size:12px}
.btn.scan:hover{background:#00b89444}
.toolbar{padding:12px 14px;display:flex;gap:10px;align-items:center;border-bottom:1px solid #2a2d3a}
.modal-bg{position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,.6);display:none;z-index:100;justify-content:center;align-items:center}
.modal-bg.show{display:flex}
.modal{background:#1a1d27;border:1px solid #2a2d3a;border-radius:8px;padding:24px;width:420px;max-width:90vw}
.modal h3{margin-bottom:16px;font-size:15px;color:#fff}
.modal label{display:block;font-size:11px;text-transform:uppercase;letter-spacing:1px;color:#666;margin:10px 0 4px}
.modal input,.modal select{width:100%;padding:8px 10px;background:#0f1117;border:1px solid #2a2d3a;border-radius:4px;color:#e0e0e0;font-size:13px}
.modal input:focus,.modal select:focus{outline:none;border-color:#6c5ce7}
.modal .btns{display:flex;gap:8px;margin-top:18px;justify-content:flex-end}
.scan-row{background:#00b89411!important}
.scan-row.unknown{background:#f39c1211!important}
</style>
</head>
<body>
<div class="hdr">
  <h1><span>&#9889;</span> Throttle Engine</h1>
  <div class="st"><div class="dot"></div>Engine running &middot; auto-refresh 10s</div>
</div>
<div class="ctn">
  <div class="stats" id="stats">
    <div class="sc"><div class="lb">Total Alerts</div><div class="vl t" id="s-total">0</div></div>
    <div class="sc"><div class="lb">Firing</div><div class="vl f" id="s-firing">0</div></div>
    <div class="sc"><div class="lb">Resolved</div><div class="vl r" id="s-resolved">0</div></div>
    <div class="sc"><div class="lb">Devices</div><div class="vl" id="s-devices">0</div></div>
    <div class="sc"><div class="lb">Uptime</div><div class="vl" id="s-uptime">-</div></div>
  </div>
  <div class="sec">
    <div class="tabs">
      <div class="tab a" onclick="switchTab(this,'alerts')">Alert History</div>
      <div class="tab" onclick="switchTab(this,'devices')">Device Inventory</div>
      <div class="tab" onclick="switchTab(this,'scan')">Network Scan</div>
    </div>
    <div class="tc a" id="tab-alerts">
      <div style="max-height:500px;overflow-y:auto">
      <table><thead><tr><th>Time</th><th>Alert</th><th>Status</th><th>Severity</th><th>Device</th><th>Owner</th><th>Summary</th></tr></thead>
      <tbody id="alert-body"><tr><td colspan="7" class="empty">No alerts yet. Engine is listening for Alertmanager webhooks.</td></tr></tbody></table>
      </div>
    </div>
    <div class="tc" id="tab-devices">
      <div class="toolbar"><button class="btn p" onclick="openModal()">+ Add Device</button><span style="font-size:11px;color:#555" id="dev-count"></span></div>
      <div class="dg" id="dev-grid"><div class="empty">Loading...</div></div>
    </div>
    <div class="tc" id="tab-scan">
      <div class="toolbar"><button class="btn scan" onclick="runScan()" id="scan-btn">Scan Network (192.168.1.0/24)</button><span style="font-size:11px;color:#555" id="scan-status"></span></div>
      <div style="max-height:500px;overflow-y:auto">
      <table><thead><tr><th>IP</th><th>MAC</th><th>Status</th><th>Known</th><th>Name</th><th>Action</th></tr></thead>
      <tbody id="scan-body"><tr><td colspan="6" class="empty">Click "Scan Network" to discover devices on your LAN.</td></tr></tbody></table>
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
let devices={};
function switchTab(el,name){document.querySelectorAll('.tab').forEach(t=>t.classList.remove('a'));document.querySelectorAll('.tc').forEach(t=>t.classList.remove('a'));el.classList.add('a');document.getElementById('tab-'+name).classList.add('a')}
function ts(d){const s=Math.floor((Date.now()-new Date(d))/1000);if(s<60)return s+'s ago';if(s<3600)return Math.floor(s/60)+'m ago';if(s<86400)return Math.floor(s/3600)+'h ago';return Math.floor(s/86400)+'d ago'}
function ut(d){const s=Math.floor((Date.now()-new Date(d))/1000);const h=Math.floor(s/3600),m=Math.floor((s%3600)/60);return h+'h '+m+'m'}
function sb(s){return s==='firing'?'<span class="bf">FIRING</span>':'<span class="br">RESOLVED</span>'}
function svb(s){if(!s)return'-';return s==='critical'?'<span class="bc">'+s+'</span>':'<span class="bw">'+s+'</span>'}

function openModal(ip,dev){
  document.getElementById('modal-bg').classList.add('show');
  document.getElementById('modal-title').textContent=ip?'Edit Device':'Add Device';
  document.getElementById('m-ip').value=ip||'';
  document.getElementById('m-ip').readOnly=!!ip;
  if(dev){document.getElementById('m-name').value=dev.name||'';document.getElementById('m-owner').value=dev.owner||'';document.getElementById('m-type').value=dev.type||'unknown';document.getElementById('m-mac').value=dev.mac||'';document.getElementById('m-notes').value=dev.notes||''}
  else{document.getElementById('m-name').value='';document.getElementById('m-owner').value='';document.getElementById('m-type').value='unknown';document.getElementById('m-mac').value='';document.getElementById('m-notes').value=''}
}
function closeModal(){document.getElementById('modal-bg').classList.remove('show')}

async function saveDevice(){
  const body={ip:document.getElementById('m-ip').value,name:document.getElementById('m-name').value,owner:document.getElementById('m-owner').value,type:document.getElementById('m-type').value,mac:document.getElementById('m-mac').value,notes:document.getElementById('m-notes').value};
  if(!body.ip||!body.name){alert('IP and Name required');return}
  await fetch('/api/v1/devices/update',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  closeModal();refresh()
}

async function deleteDevice(ip){
  if(!confirm('Remove '+ip+' from inventory?'))return;
  await fetch('/api/v1/devices/update',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({ip})});
  refresh()
}

async function runScan(){
  const btn=document.getElementById('scan-btn');btn.textContent='Scanning...';btn.disabled=true;
  document.getElementById('scan-status').textContent='Running nmap -sn 192.168.1.0/24...';
  try{
    const r=await fetch('/api/v1/scan');const hosts=await r.json();
    const tb=document.getElementById('scan-body');
    if(!hosts||hosts.length===0){tb.innerHTML='<tr><td colspan="6" class="empty">No hosts found.</td></tr>';return}
    tb.innerHTML=hosts.map(h=>{
      const cls=h.known?'scan-row':'scan-row unknown';
      const knownBadge=h.known?'<span class="br">Known</span>':'<span class="bw">Unknown</span>';
      const name=h.device?h.device.name:'-';
      const action=h.known?'<button class="btn" onclick=\'openModal("'+h.ip+'",devices["'+h.ip+'"])\'>Edit</button>':'<button class="btn p" onclick=\'openModal("'+h.ip+'",{mac:"'+(h.mac||'')+'"})\'>Add to Inventory</button>';
      return '<tr class="'+cls+'"><td style="font-family:monospace">'+h.ip+'</td><td style="font-family:monospace;font-size:11px;color:#666">'+(h.mac||'-')+'</td><td><span class="bu">'+h.state+'</span></td><td>'+knownBadge+'</td><td>'+name+'</td><td>'+action+'</td></tr>'
    }).join('');
    document.getElementById('scan-status').textContent=hosts.length+' hosts found';
  }catch(e){document.getElementById('scan-status').textContent='Scan failed: '+e}
  finally{btn.textContent='Scan Network (192.168.1.0/24)';btn.disabled=false}
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
    document.getElementById('dev-count').textContent=ips.length+' devices in inventory';

    const tb=document.getElementById('alert-body');
    if(alerts&&alerts.length>0){tb.innerHTML=alerts.map(a=>'<tr><td style="white-space:nowrap">'+ts(a.timestamp)+'</td><td>'+
      (a.alert_name||'-')+'</td><td>'+sb(a.status)+'</td><td>'+svb(a.severity)+'</td><td>'+(a.device_name||a.instance||'-')+
      '</td><td>'+(a.owner||'-')+'</td><td style="max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+(a.summary||'-')+'</td></tr>').join('')}
    else{tb.innerHTML='<tr><td colspan="7" class="empty">No alerts yet. Engine is listening for Alertmanager webhooks.</td></tr>'}

    const grid=document.getElementById('dev-grid');
    if(ips.length>0){grid.innerHTML=ips.sort().map(ip=>{const d=devices[ip];
      const typeBadge='<span class="bd">'+d.type+'</span>';
      return '<div class="dc"><div class="acts"><button class="btn" onclick=\'openModal("'+ip+'",devices["'+ip+'"])\'>Edit</button><button class="btn d" onclick="deleteDevice(\''+ip+'\')">Del</button></div>'+
        '<div class="ip">'+ip+'</div><div class="nm">'+d.name+'</div>'+
        '<div class="mt">'+d.owner+' &middot; '+typeBadge+'</div>'+
        (d.mac?'<div class="mac">'+d.mac+'</div>':'')+
        (d.notes?'<div class="mt" style="margin-top:6px;font-style:italic">'+d.notes+'</div>':'')+
        '</div>'}).join('')}
    else{grid.innerHTML='<div class="empty">No devices. Add some or run a network scan.</div>'}
  }catch(e){console.error('refresh failed',e)}
}
refresh();setInterval(refresh,10000);
</script>
</body>
</html>`
