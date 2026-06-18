// Package web serves a minimal local dashboard for Peapod over HTTP.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"peapod/internal/sandbox"
)

// Serve runs the dashboard on addr until ctx is cancelled.
func Serve(ctx context.Context, mgr *sandbox.Manager, addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	})

	mux.HandleFunc("/api/sandboxes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			boxes, err := mgr.List(r.Context())
			if err != nil {
				writeErr(w, err)
				return
			}
			if boxes == nil {
				boxes = []sandbox.Sandbox{}
			}
			writeJSON(w, map[string]any{"sandboxes": boxes})
		case http.MethodPost:
			var in struct {
				Image   string `json:"image"`
				Network string `json:"network"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeErr(w, err)
				return
			}
			sb, err := mgr.Create(r.Context(), sandbox.Spec{Image: in.Image, Network: sandbox.NetworkPolicy(in.Network)})
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, sb)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/destroy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, err)
			return
		}
		if err := mgr.Destroy(r.Context(), in.ID); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/snapshots", func(w http.ResponseWriter, r *http.Request) {
		snaps, err := mgr.ListSnapshots(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		if snaps == nil {
			snaps = []sandbox.Snapshot{}
		}
		writeJSON(w, map[string]any{"snapshots": snaps})
	})

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

const page = `<!doctype html>
<html><head><meta charset="utf-8"><title>Peapod</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
 body{font-family:-apple-system,system-ui,sans-serif;margin:2rem;color:#1a1a1a;background:#fafafa}
 h1{font-weight:600}
 h2{font-weight:500;margin-top:2rem;font-size:1.05rem}
 table{border-collapse:collapse;width:100%;background:#fff;border:1px solid #e5e5e5;border-radius:8px;overflow:hidden}
 th,td{text-align:left;padding:.5rem .75rem;border-bottom:1px solid #f0f0f0;font-size:.9rem}
 th{background:#f7f7f7;font-weight:500}
 button{font:inherit;padding:.35rem .7rem;border:1px solid #ccc;border-radius:6px;background:#fff;cursor:pointer}
 button.danger{color:#a32d2d;border-color:#e0b4b4}
 button.primary{background:#1a1a1a;color:#fff;border-color:#1a1a1a}
 form{display:flex;gap:.5rem;align-items:center;margin:.75rem 0}
 input,select{font:inherit;padding:.35rem .5rem;border:1px solid #ccc;border-radius:6px}
 .muted{color:#888;font-size:.85rem}
 .pill{font-size:.75rem;padding:.1rem .45rem;border-radius:999px;background:#eee}
</style></head>
<body>
 <h1>Peapod</h1>
 <form id="createForm">
   <input id="image" placeholder="image (e.g. alpine)" value="alpine" required>
   <select id="network"><option value="none">network: none</option><option value="egress">network: egress</option></select>
   <button class="primary" type="submit">create sandbox</button>
 </form>
 <h2>sandboxes</h2>
 <table><thead><tr><th>id</th><th>image</th><th>network</th><th>name</th><th></th></tr></thead><tbody id="boxes"></tbody></table>
 <h2>snapshots</h2>
 <table><thead><tr><th>ref</th><th>name</th><th>created</th><th>size</th></tr></thead><tbody id="snaps"></tbody></table>
 <script>
 function esc(s){return (s||"").replace(/[&<>"]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c]})}
 async function refresh(){
   try{
     var d = await (await fetch("/api/sandboxes")).json();
     var rows = (d.sandboxes||[]).map(function(b){
       return "<tr><td>"+esc(b.id)+"</td><td>"+esc(b.image)+"</td><td><span class=pill>"+esc(b.network)+"</span></td><td>"+esc(b.name)+"</td>"+
         "<td><button class=danger onclick=\"destroy('"+esc(b.id)+"')\">destroy</button></td></tr>";
     }).join("");
     document.getElementById("boxes").innerHTML = rows || "<tr><td colspan=5 class=muted>no sandboxes</td></tr>";
     var d2 = await (await fetch("/api/snapshots")).json();
     var rows2 = (d2.snapshots||[]).map(function(s){
       return "<tr><td>"+esc(s.ref)+"</td><td>"+esc(s.name)+"</td><td>"+esc(s.created)+"</td><td>"+esc(s.size)+"</td></tr>";
     }).join("");
     document.getElementById("snaps").innerHTML = rows2 || "<tr><td colspan=4 class=muted>no snapshots</td></tr>";
   }catch(e){}
 }
 async function destroy(id){
   await fetch("/api/destroy",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({id:id})});
   refresh();
 }
 document.getElementById("createForm").addEventListener("submit", async function(e){
   e.preventDefault();
   await fetch("/api/sandboxes",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({image:document.getElementById("image").value,network:document.getElementById("network").value})});
   refresh();
 });
 refresh(); setInterval(refresh, 3000);
 </script>
</body></html>`
