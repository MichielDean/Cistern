package main

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/MichielDean/cistern/internal/cistern"
)

// wsWriteTimeout is the per-frame write deadline set on the hijacked net.Conn
// before each wsSendText call. Without this, a client that disappears via a
// network partition (no TCP FIN) causes the goroutine to block indefinitely
// inside bufio.Writer.Flush.
const wsWriteTimeout = 10 * time.Second

// aqueductSessionInfo holds the tmux session name and droplet context for an
// active aqueduct worker.
type aqueductSessionInfo struct {
	sessionID string
	dropletID string
	title     string
	elapsed   time.Duration
}

// lookupAqueductSession returns session info for the named aqueduct worker, or
// false if the worker is not currently flowing.
func lookupAqueductSession(dbPath, name string) (aqueductSessionInfo, bool) {
	c, err := cistern.New(dbPath, "")
	if err != nil {
		return aqueductSessionInfo{}, false
	}
	defer c.Close()

	items, err := c.List("", "in_progress")
	if err != nil {
		return aqueductSessionInfo{}, false
	}
	for _, item := range items {
		if item.Assignee == name {
			return aqueductSessionInfo{
				sessionID: item.Repo + "-" + name,
				dropletID: item.ID,
				title:     item.Title,
				elapsed:   time.Since(item.UpdatedAt),
			}, true
		}
	}
	return aqueductSessionInfo{}, false
}

// parsePeekLines reads the optional ?lines= query parameter, falling back to
// defaultPeekLines.
func parsePeekLines(r *http.Request) int {
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultPeekLines
}

// wsAcceptKey computes Sec-WebSocket-Accept per RFC 6455 §4.2.2.
func wsAcceptKey(clientKey string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	h.Write([]byte(clientKey + magic))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// wsSendText writes a WebSocket text frame to the buffered writer and flushes.
// The server never masks frames (RFC 6455 §5.1).
func wsSendText(w *bufio.Writer, data string) error {
	payload := []byte(data)
	n := len(payload)
	header := make([]byte, 0, 10)
	header = append(header, 0x81) // FIN=1, opcode=0x1 (text)
	switch {
	case n < 126:
		header = append(header, byte(n))
	case n < 65536:
		header = append(header, 0x7E)
		header = binary.BigEndian.AppendUint16(header, uint16(n))
	default:
		header = append(header, 0x7F)
		header = binary.BigEndian.AppendUint64(header, uint64(n))
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return w.Flush()
}

// wsUpgrade performs the RFC 6455 handshake. On success it returns the hijacked
// connection and its buffered read-writer. On failure it writes an HTTP error
// and returns a non-nil error.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return nil, nil, fmt.Errorf("not a websocket request")
	}
	key := r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, nil, fmt.Errorf("missing Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return nil, nil, fmt.Errorf("hijacking not supported")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAcceptKey(key) + "\r\n" +
		"\r\n"
	if _, err := brw.WriteString(resp); err != nil {
		conn.Close()
		return nil, nil, err
	}
	if err := brw.Flush(); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, brw, nil
}

// newDashboardMux returns an http.Handler for the web dashboard.
// Exposed for testing.
func newDashboardMux(cfgPath, dbPath string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dashboardHTML)
	})

	mux.HandleFunc("/api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data := fetchDashboardData(cfgPath, dbPath)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data) //nolint:errcheck
	})

	mux.HandleFunc("/api/dashboard/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		send := func() {
			data := fetchDashboardData(cfgPath, dbPath)
			b, err := json.Marshal(data)
			if err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}

		send()

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				send()
			}
		}
	})

	// GET /api/aqueducts/{name}/peek — snapshot of current tmux pane output.
	mux.HandleFunc("/api/aqueducts/{name}/peek", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := r.PathValue("name")
		lines := parsePeekLines(r)
		sess, ok := lookupAqueductSession(dbPath, name)
		capturer := defaultCapturer
		if !ok || !capturer.HasSession(sess.sessionID) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprint(w, "session not active")
			return
		}
		content, err := capturer.Capture(sess.sessionID, lines)
		if err != nil {
			http.Error(w, fmt.Sprintf("capture error: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, stripANSI(content))
	})

	// WS /ws/aqueducts/{name}/peek — live streaming peek (poll every 500ms, send diffs).
	mux.HandleFunc("/ws/aqueducts/{name}/peek", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		lines := parsePeekLines(r)

		conn, brw, err := wsUpgrade(w, r)
		if err != nil {
			return // wsUpgrade already wrote the HTTP error
		}
		defer conn.Close()

		var prev string
		capturer := defaultCapturer
		ticker := time.NewTicker(peekInterval)
		defer ticker.Stop()

		for range ticker.C {
			next := "session not active"
			if sess, ok := lookupAqueductSession(dbPath, name); ok && capturer.HasSession(sess.sessionID) {
				content, err := capturer.Capture(sess.sessionID, lines)
				if err != nil {
					continue
				}
				next = stripANSI(content)
			}
			if diff := computeDiff(prev, next); diff != "" {
				conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)) //nolint:errcheck
				if wsSendText(brw.Writer, diff) != nil {
					return
				}
				prev = next
			}
		}
	})

	return mux
}

// RunDashboardWeb starts the HTTP web dashboard on addr and blocks until
// SIGINT/SIGTERM is received or the server fails.
func RunDashboardWeb(cfgPath, dbPath, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           newDashboardMux(cfgPath, dbPath),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      0, // SSE streams are long-lived
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "Cistern web dashboard listening on http://localhost%s\n", addr)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// dashboardHTML is the single-page web dashboard — a faithful pre-based port
// of the TUI (dashboard_tui.go). Animation loop runs at 150ms (animInterval).
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cistern</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0d1117;color:#e6edf3;font-family:'Cascadia Code','Courier New',Courier,monospace;font-size:13px;line-height:1.3;padding:0}
#conn{font-size:11px;padding:3px 8px;color:#e06c75}
#conn.live{color:#4bb96e}
#screen{padding:4px 8px 8px;white-space:pre;overflow-x:auto;cursor:default}
.peek-overlay{position:fixed;inset:0;background:rgba(0,0,0,.65);display:none;z-index:100;align-items:center;justify-content:center}
.peek-overlay.open{display:flex}
.peek-panel{background:#161b22;border:1px solid #30363d;border-radius:4px;width:90vw;max-width:900px;height:70vh;display:flex;flex-direction:column;overflow:hidden}
.peek-hdr{padding:6px 10px;border-bottom:1px solid #30363d;display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.peek-ro-label{color:#4bb96e;font-size:11px;font-weight:bold;white-space:nowrap}
.peek-title{flex:1;font-size:12px;color:#7d8590;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.peek-btn{background:none;border:1px solid #30363d;color:#7d8590;font-size:11px;padding:2px 6px;cursor:pointer;border-radius:3px;font-family:inherit}
.peek-btn:hover{color:#e6edf3;border-color:#7d8590}
.peek-content{flex:1;overflow-y:auto;padding:8px;font-size:12px;white-space:pre-wrap;word-break:break-all;color:#e6edf3;background:#0d1117}
.peek-footer{padding:3px 10px;border-top:1px solid #30363d;font-size:11px;color:#7d8590}
</style>
</head>
<body>
<div id="conn">&#x25CB; connecting&#x2026;</div>
<pre id="screen"></pre>
<div id="peek-overlay" class="peek-overlay">
  <div class="peek-panel">
    <div class="peek-hdr">
      <span class="peek-ro-label">Observing &#x2014; read only</span>
      <span id="peek-title" class="peek-title"></span>
      <button class="peek-btn" id="peek-pin-btn" onclick="peekTogglePin()">pin scroll</button>
      <button class="peek-btn" onclick="peekClose()">&#x2715; close</button>
    </div>
    <div id="peek-content" class="peek-content">(connecting&#x2026;)</div>
    <div id="peek-footer" class="peek-footer">connecting&#x2026;</div>
  </div>
</div>
<script>
var screenEl=document.getElementById('screen');
var connEl=document.getElementById('conn');

// TUI palette — mirrors dashboard_tui.go style vars exactly.
var cDim='#46465a',cGreen='#4bb96e',cYellow='#f0c86b',cRed='#e06c75';
var cHeader='#9db1db',cFoot='#36364a';
var wfBr='#a8eeff',wfMd='#3ec8e8',wfDm='#1a7a96';

// Arch constants — must match dashboard_tui.go tuiAqueductRow constants.
var COL_W=20,ARCH_TOP=10,TAPER=3,PIER_ROWS=1,BRICK=4,NAME_W=10;
var PIER_W=ARCH_TOP-TAPER*2; // 4
var SCR_W=120; // screen width (chars)

function esc(s){
  if(s==null)return'';
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// Wrap text in a coloured span. text is HTML-escaped inside.
function sp(color,text,bold){
  if(text==null||text==='')return'';
  var st='color:'+color;
  if(bold)st+=';font-weight:bold';
  return'<span style="'+st+'">'+esc(text)+'</span>';
}

// Pad string to width w (codepoint-aware).
function padR(s,w){
  s=String(s||'');
  var r=Array.from(s);
  if(r.length>=w)return r.slice(0,w).join('');
  return s+' '.repeat(w-r.length);
}

// Centre string in width w, truncate with ellipsis if too long.
function padC(s,w){
  var r=Array.from(s);
  if(r.length>w)return r.slice(0,w-1).join('')+'\u2026';
  var tot=w-r.length,l=Math.floor(tot/2),ri=tot-l;
  return' '.repeat(l)+s+' '.repeat(ri);
}

function pbar(idx,tot,w){
  if(tot<=0||idx<=0)return'\u2591'.repeat(w);
  var f=Math.min(Math.floor(idx*w/tot),w);
  return'\u2588'.repeat(f)+'\u2591'.repeat(w-f);
}

function fmtNs(ns){
  var s=Math.floor(ns/1e9);
  if(s<0)return'0s';
  if(s<60)return s+'s';
  var m=Math.floor(s/60);
  return m+'m '+(s%60)+'s';
}

// Relative timestamp: mirrors viewCurrentFlow note timestamp logic in TUI.
function relAge(iso){
  if(!iso)return'';
  var age=Math.floor((Date.now()-new Date(iso).getTime())/1000);
  if(age<60)return'just now';
  if(age<3600)return Math.floor(age/60)+'m ago';
  return new Date(iso).toLocaleTimeString([],{hour:'2-digit',minute:'2-digit'});
}

// Truncate string to n codepoints, appending ellipsis.
function trunc(s,n){
  var r=Array.from(s||'');
  if(r.length<=n)return s||'';
  return r.slice(0,n-1).join('')+'\u2026';
}

// First non-empty, non-comment line from multi-line note content.
// Mirrors firstMeaningfulLine() in dashboard_tui.go.
function firstLine(txt){
  var ls=(txt||'').split('\n');
  for(var i=0;i<ls.length;i++){
    var l=ls[i].trim();
    if(l&&l.charAt(0)!=='#'&&l.indexOf('---')!==0)return l;
  }
  return(txt||'').trim();
}

// ── Wave animation ────────────────────────────────────────────────────────────
// waveCells mirrors the waveCells slice in tuiAqueductRow.
var WV=[
  {ch:'\u2591',col:wfDm},{ch:'\u2592',col:wfMd},{ch:'\u2593',col:wfBr},
  {ch:'\u2248',col:wfMd},{ch:'\u2592',col:wfMd},{ch:'\u2591',col:wfDm}
];
// renderWave mirrors renderWave() closure in tuiAqueductRow.
function renderWave(n,fr){
  var s='',len=WV.length;
  for(var i=0;i<n;i++){
    var c=WV[((i-fr%len)+len*1000)%len];
    s+=sp(c.col,c.ch);
  }
  return s;
}

// chanWater mirrors buildChanWater() closure in tuiAqueductRow.
function chanWater(info,iCol,cW,fr){
  var iv=Array.from(info).length;
  var sw=Math.floor((cW-2-iv)/2);if(sw<0)sw=0;
  var rw=cW-2-iv-sw;if(rw<0)rw=0;
  return renderWave(sw,fr)+sp(iCol,info)+renderWave(rw,fr);
}

// ── Arch crown (semicircle formula) ──────────────────────────────────────────
// Mirrors archCrownAtT() closure in tuiAqueductRow.
function archCrown(t,gW){
  if(gW<=0)return[0,0,0];
  var r=gW/2,oh=r*Math.sin(Math.PI/2*t),fe=r-oh;
  var full=Math.floor(fe),frac=fe-full;
  var haunch=frac>0.25&&gW>2;
  var lf=full+(haunch?1:0),rf=lf,og=gW-lf-rf;
  if(og<0){og=0;lf=Math.floor(gW/2);rf=gW-lf;}
  return[lf,og,rf];
}

// ── Waterfall (Option C: spill & curtain) ────────────────────────────────────
// wfCol mirrors wfA() closure in tuiAqueductRow (brightness rotates with frame).
function wfCol(sub,fr){
  switch((sub+fr)%3){case 0:return wfBr;case 1:return wfMd;default:return wfDm;}
}

// buildWfRows mirrors the wfRows [8]string array in tuiAqueductRow.
function buildWfRows(fr){
  var p=' ',pp='  ';
  return[
    sp(wfMd,'\u2592')+sp(wfCol(0,fr),'\u2593')+sp(wfMd,'\u2592')+sp(wfDm,'\u2591'),
    sp(wfDm,'\u2591')+sp(wfCol(1,fr),'\u2593')+sp(wfMd,'\u2592'),
    p+sp(wfMd,'\u2592')+sp(wfCol(2,fr),'\u2593')+sp(wfMd,'\u2592'),
    p+sp(wfDm,'\u2591')+sp(wfCol(0,fr),'\u2593')+sp(wfMd,'\u2592'),
    pp+sp(wfCol(1,fr),'\u2593')+sp(wfMd,'\u2592'),
    pp+sp(wfCol(2,fr),'\u2593')+sp(wfMd,'\u2592'),
    pp+sp(wfDm,'\u2591')+sp(wfMd,'\u2592')+sp(wfCol(0,fr),'\u2593')+sp(wfMd,'\u2592')+sp(wfDm,'\u2591'),
    sp(wfDm,'\u2591\u2248')+sp(wfMd,'\u2592\u2592')+sp(wfCol(1,fr),'\u2593\u2593')+sp(wfMd,'\u2592\u2592')+sp(wfDm,'\u2248\u2591')
  ];
}

// ── Active aqueduct: full Roman arch diagram ──────────────────────────────────
// Mirrors tuiAqueductRow() in dashboard_tui.go exactly.
// Returns an array of HTML line strings (l1, l2, arch sub-rows..., label).
function aqRow(ch,fr){
  var steps=(ch.steps&&ch.steps.length)?ch.steps:['\u2014'];
  var n=steps.length;
  function actv(s){return s===ch.step&&!!ch.droplet_id;}

  var pTxt='  '+padR(ch.name,NAME_W)+'  ';
  var indent=' '.repeat(pTxt.length);
  var repo=trunc(ch.repo_name||'',NAME_W);
  var pRepo='  '+sp(cDim,padR(repo,NAME_W))+'  ';

  var cW=n*COL_W;

  // l1: aqueduct name (unstyled white) + mortar cap row (dim).
  // Mirrors: prefix + cStyle.Render(strings.Repeat("\u2580", chanW))
  var l1=esc(pTxt)+sp(cDim,'\u2580'.repeat(cW));

  // Water content for l2.
  var water;
  if(ch.droplet_id){
    var bar=pbar(ch.cataractae_index,ch.total_cataractae,8);
    if(ch.note_count>0){
      water=chanWater('  \u267b '+ch.droplet_id+'  '+fmtNs(ch.elapsed)+'  '+bar+'  ',cYellow,cW,fr);
    }else{
      water=chanWater('  '+ch.droplet_id+'  '+fmtNs(ch.elapsed)+'  '+bar+'  ',wfMd,cW,fr);
    }
  }else{
    water=chanWater('  \u2014 idle \u2014  ',cDim,cW,fr);
  }

  var wfR=buildWfRows(fr);
  // wfExit mirrors: wfDim.Render("\u2591")+wfMid.Render("\u2592")+wfA(0).Render("\u2593\u2593")
  var wfExit=sp(wfDm,'\u2591')+sp(wfMd,'\u2592')+sp(wfCol(0,fr),'\u2593\u2593');

  // l2: repo prefix + channel wall + water + channel wall + waterfall exit.
  // Channel row is clickable when active (opens peek).
  var l2chan=sp(cDim,'\u2588')+water+sp(cDim,'\u2588')+wfExit;
  var l2;
  if(ch.droplet_id){
    l2=pRepo+'<span data-aqname="'+esc(ch.name)+'" style="cursor:pointer">'+l2chan+'</span>';
  }else{
    l2=pRepo+l2chan;
  }

  // Arch + pier rows: TAPER*2 + PIER_ROWS*2 rendered sub-rows.
  // Each logical row lr produces a mortar sub-row and a brick sub-row.
  var archLines=[];
  for(var lr=0;lr<TAPER+PIER_ROWS;lr++){
    var bW=Math.max(ARCH_TOP-lr*2,PIER_W);
    var rPL=Math.floor((COL_W-bW)/2);
    var gW=COL_W-bW;

    var tM=Math.min(lr/TAPER,1.0);
    var crM=(lr<TAPER)?archCrown(tM,gW):[0,gW,0];
    var tB=Math.min(lr+0.5,TAPER)/TAPER;
    var crB=(lr<TAPER)?archCrown(tB,gW):[0,gW,0];
    var lfM=crM[0],ogM=crM[1],rfM=crM[2];
    var lfB=crB[0],ogB=crB[1],rfB=crB[2];

    var mSB=indent,bSB=indent;
    // Brick offset alternates rows for staggered courses.
    var off=Math.floor(BRICK/2)*(lr%2);

    // Left abutment.
    var aM='\u2580'.repeat(rPL),aB='';
    for(var cc=0;cc<rPL;cc++)aB+=((cc+off)%(BRICK+1)===BRICK)?'\u258c':'\u2588';
    mSB+=sp(cDim,aM);bSB+=sp(cDim,aB);

    for(var i=0;i<n;i++){
      var step=steps[i],pC=actv(step)?cGreen:cDim;
      // Pier mortar sub-row.
      mSB+=sp(pC,'\u2580'.repeat(bW));
      // Pier brick sub-row: staggered joint positions.
      var bd='';
      for(var cc=0;cc<bW;cc++)bd+=((cc+off)%(BRICK+1)===BRICK)?'\u258c':'\u2588';
      bSB+=sp(pC,bd);

      // Inter-pier span: arch crown fill with per-side colour attribution.
      if(i<n-1){
        var lC=actv(step)?cGreen:cDim,rC=actv(steps[i+1])?cGreen:cDim;
        // Mortar sub-row arch crown.
        if(lfM>0)mSB+=sp(lC,'\u2580'.repeat(lfM));
        if(ogM>0)mSB+=' '.repeat(ogM);
        if(rfM>0)mSB+=sp(rC,'\u2580'.repeat(rfM));
        // Brick sub-row arch crown with haunch details.
        if(lfB>0){if(lfB>1)bSB+=sp(lC,'\u2588'.repeat(lfB-1));bSB+=sp(lC,'\u258c');}
        if(ogB>0)bSB+=' '.repeat(ogB);
        if(rfB>0){bSB+=sp(rC,'\u2590');if(rfB>1)bSB+=sp(rC,'\u2588'.repeat(rfB-1));}
      }
    }

    // Right abutment (same as left).
    mSB+=sp(cDim,aM);bSB+=sp(cDim,aB);

    // Append waterfall sub-row pair.
    var sr=lr*2;
    mSB+=wfR[sr];bSB+=wfR[sr+1];
    archLines.push(mSB,bSB);
  }

  // Label line: step names centred under each pier column.
  var lbl=indent;
  for(var i=0;i<steps.length;i++){
    var step=steps[i],lb=trunc(step,COL_W-1);
    lbl+=actv(step)?sp(cGreen,padC(lb,COL_W),true):sp(cDim,padC(lb,COL_W));
  }

  var res=[l1,l2];
  res=res.concat(archLines);
  res.push(lbl);
  return res;
}

// ── Idle aqueduct: compact single dim line ────────────────────────────────────
// Mirrors viewIdleAqueductRow() in dashboard_tui.go.
function idleRow(ch){
  var nW=12,rW=18;
  return'  '+sp(cDim,padR(ch.name,nW))+'  '+sp(cDim,padR(ch.repo_name||'',rW))+'  '+sp(cDim,'\u00b7  idle');
}

// ── Aqueduct section ──────────────────────────────────────────────────────────
// Mirrors viewAqueductArches(): active arches first, then compact idle rows.
function viewArches(d,fr){
  var chs=d.cataractae||[];
  if(!chs.length)return[sp(cDim,'  No aqueducts configured')];
  var active=[],idle=[];
  for(var i=0;i<chs.length;i++){
    (chs[i].droplet_id?active:idle).push(chs[i]);
  }
  var lines=[];
  for(var i=0;i<active.length;i++){
    if(i>0)lines.push('');
    var rows=aqRow(active[i],fr);
    for(var j=0;j<rows.length;j++)lines.push(rows[j]);
  }
  if(idle.length){
    if(active.length)lines.push('');
    for(var i=0;i<idle.length;i++)lines.push(idleRow(idle[i]));
  }
  return lines;
}

// ── CURRENT FLOW with relative timestamps ────────────────────────────────────
// Mirrors viewCurrentFlow() in dashboard_tui.go.
function viewCurrentFlow(d){
  var acts=d.flow_activities||[];
  if(!acts.length)return[sp(cDim,'  No droplets currently flowing.')];
  var lines=[];
  for(var i=0;i<acts.length;i++){
    var fa=acts[i];
    var rtag='',hC=cGreen;
    if(fa.note_count>0){rtag=sp(cYellow,' \u267b '+fa.note_count);hC=cYellow;}
    lines.push('  '+sp(cHeader,fa.droplet_id,true)+'  '+sp(hC,fa.step)+rtag+sp(cDim,'  '+trunc(fa.title||'',60)));
    var notes=fa.recent_notes||[];
    if(!notes.length){
      lines.push(sp(cDim,'    (no notes yet \u2014 first pass)'));
    }else{
      for(var k=0;k<notes.length;k++){
        var nt=notes[k];
        var ts=relAge(nt.created_at);
        var txt=trunc(firstLine(nt.content),80);
        lines.push('    \u203a '+sp(cDim,'['+(nt.cataractae_name||'')+']')+'  '+sp(cFoot,txt)+'  '+sp(cDim,ts));
      }
    }
    lines.push('');
  }
  if(lines.length&&lines[lines.length-1]==='')lines.pop();
  return lines;
}

// ── CISTERN with priority icons ───────────────────────────────────────────────
// Mirrors viewCisternRow() in dashboard_tui.go.
function viewCistern(d){
  var items=(d.cistern_items||[]).filter(function(x){return x.status==='open';});
  if(!items.length)return[sp(cDim,'  Cistern is empty.')];
  var lines=[];
  for(var i=0;i<items.length;i++){
    var it=items[i];
    var age=Math.max(0,Math.floor((Date.now()-new Date(it.created_at||0).getTime())/1000));
    var bl=d.blocked_by_map&&d.blocked_by_map[it.id];
    var st=bl?sp(cRed,'blocked by '+bl):sp(cYellow,'queued');
    var pr;
    switch(it.priority){case 1:pr=sp(cRed,'\u2191');break;case 3:pr=sp(cDim,'\u2193');break;default:pr=sp(cDim,'\u00b7');}
    lines.push('  '+pr+' '+sp(cDim,padR(it.id,10))+'  '+sp(cDim,fmtNs(age*1e9))+'  '+st+'  '+sp(cDim,trunc(it.title||'',50)));
  }
  return lines;
}

// ── RECENT FLOW ───────────────────────────────────────────────────────────────
// Mirrors viewRecentRow() in dashboard_tui.go.
function viewRecent(d){
  var items=d.recent_items||[];
  if(!items.length)return[sp(cDim,'  No recent flow.')];
  var lines=[];
  for(var i=0;i<items.length;i++){
    var it=items[i];
    var t=it.updated_at?new Date(it.updated_at).toLocaleTimeString([],{hour:'2-digit',minute:'2-digit'}):'';
    var step=it.current_cataractae||'\u2014';
    var icon;
    switch(it.status){case'delivered':icon=sp(cGreen,'\u2713');break;case'stagnant':icon=sp(cRed,'\u2717');break;default:icon=sp(cDim,'\u00b7');}
    lines.push('  '+sp(cDim,t)+'  '+sp(cDim,padR(it.id,10))+'  '+sp(cDim,padR(step,20))+'  '+icon+'  '+sp(cDim,trunc(it.title||'',40)));
  }
  return lines;
}

// ── Main render ───────────────────────────────────────────────────────────────
var dashData=null,animFr=0;
function sepLine(){return sp(cDim,'\u2500'.repeat(SCR_W));}

function render(){
  if(!dashData){screenEl.innerHTML=sp(cDim,'  Loading\u2026');return;}
  var d=dashData;
  var lines=[];

  // Logo header — mirrors viewLogo().
  lines.push(sp(cDim,'\u2593'.repeat(SCR_W)));
  lines.push(sp(cHeader,padC('\u25c8  C I S T E R N  \u25c8',SCR_W),true));
  lines.push(sp(cDim,'\u2593'.repeat(SCR_W)));
  lines.push(sepLine());

  // Aqueduct arch diagrams.
  var aqL=viewArches(d,animFr);
  for(var i=0;i<aqL.length;i++)lines.push(aqL[i]);
  lines.push(sepLine());

  // Status bar — mirrors viewStatusBar().
  var ts=d.fetched_at?new Date(d.fetched_at).toLocaleTimeString():'';
  lines.push('  '+sp(cGreen,'\u25cf '+(d.flowing_count||0)+' flowing')+'  '+sp(cYellow,'\u25cb '+(d.queued_count||0)+' queued')+'  '+sp(cGreen,'\u2713 '+(d.done_count||0)+' delivered')+'  '+sp(cDim,'\u2014 last update '+ts));
  lines.push(sepLine());

  // Current flow.
  lines.push(sp(cHeader,'  CURRENT FLOW',true));
  var cfL=viewCurrentFlow(d);
  for(var i=0;i<cfL.length;i++)lines.push(cfL[i]);
  lines.push(sepLine());

  // Cistern queue.
  lines.push(sp(cHeader,'  CISTERN',true));
  var cqL=viewCistern(d);
  for(var i=0;i<cqL.length;i++)lines.push(cqL[i]);
  lines.push(sepLine());

  // Recent flow.
  lines.push(sp(cHeader,'  RECENT FLOW',true));
  var rfL=viewRecent(d);
  for(var i=0;i<rfL.length;i++)lines.push(rfL[i]);
  lines.push(sepLine());

  lines.push(sp(cFoot,'  last update: '+ts));
  screenEl.innerHTML=lines.join('\n');
}

// Animation loop at 150ms — matches TUI animInterval constant.
setInterval(function(){animFr++;render();},150);

// SSE connection.
function connect(){
  connEl.className='';connEl.innerHTML='&#x25CB; connecting&#x2026;';
  var es=new EventSource('/api/dashboard/events');
  es.onopen=function(){connEl.className='live';connEl.innerHTML='&#x25CF; live';};
  es.onmessage=function(e){try{dashData=JSON.parse(e.data);}catch(err){console.error('cistern:',err);}};
  es.onerror=function(){connEl.className='';connEl.innerHTML='&#x25CB; reconnecting&#x2026;';es.close();setTimeout(connect,3000);};
}
connect();

// ── Peek modal ────────────────────────────────────────────────────────────────
var peekWs=null,peekPinned=false,peekAqName='';

function peekOpen(name){
  peekAqName=name;peekPinned=false;
  document.getElementById('peek-title').textContent=name;
  document.getElementById('peek-content').textContent='(connecting\u2026)';
  document.getElementById('peek-footer').textContent='connecting\u2026';
  document.getElementById('peek-pin-btn').textContent='pin scroll';
  document.getElementById('peek-pin-btn').style.color='';
  document.getElementById('peek-overlay').classList.add('open');
  peekConnect(name);
}

function peekClose(){
  document.getElementById('peek-overlay').classList.remove('open');
  if(peekWs){peekWs.close();peekWs=null;}
  peekAqName='';
}

function peekTogglePin(){
  peekPinned=!peekPinned;
  var btn=document.getElementById('peek-pin-btn');
  btn.textContent=peekPinned?'unpin scroll':'pin scroll';
  btn.style.color=peekPinned?'#4bb96e':'';
  document.getElementById('peek-footer').textContent=peekPinned?'scroll pinned \u2014 click to unpin':'auto-scroll active';
}

function peekConnect(name){
  if(peekWs)peekWs.close();
  var proto=location.protocol==='https:'?'wss:':'ws:';
  var url=proto+'//'+location.host+'/ws/aqueducts/'+encodeURIComponent(name)+'/peek';
  var ws=new WebSocket(url);
  peekWs=ws;
  ws.onopen=function(){document.getElementById('peek-footer').textContent='Observing \u2014 read only';};
  ws.onmessage=function(e){
    if(!e.data)return;
    var el=document.getElementById('peek-content');
    el.textContent=e.data;
    if(!peekPinned)el.scrollTop=el.scrollHeight;
  };
  ws.onerror=function(){document.getElementById('peek-footer').textContent='connection error';};
  ws.onclose=function(){
    if(document.getElementById('peek-overlay').classList.contains('open')&&peekAqName){
      document.getElementById('peek-footer').textContent='disconnected \u2014 retrying in 3s\u2026';
      setTimeout(function(){if(peekAqName)peekConnect(peekAqName);},3000);
    }
  };
}

document.getElementById('peek-overlay').addEventListener('click',function(e){if(e.target===this)peekClose();});
screenEl.addEventListener('click',function(e){
  var el=e.target.closest&&e.target.closest('[data-aqname]');
  if(el)peekOpen(el.dataset.aqname);
});
</script>
</body>
</html>`
