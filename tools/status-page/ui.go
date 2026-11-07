package main

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"
)

// The public page: plain HTML, no scripts (the panel's ui:public policy allows none),
// a meta refresh keeps it current. Language: ?lang= or Accept-Language, ru by default.

type row struct {
	Title  string
	Node   string
	Status string
	Badge  string
	Up24   string
	Up7    string
	Up30   string
	Since  string
}

type pageData struct {
	Lang      string
	Banner    string
	BannerCls string
	Note      string
	Rows      []row
	Updated   string
	Checked   string
	Failed    bool
}

var pageTmpl = template.Must(template.New("page").Funcs(template.FuncMap{"statusText": statusText}).Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="60">
<title>{{if eq .Lang "ru"}}Статус Scilla{{else}}Scilla status{{end}}</title>
<style>
:root{--bg:#0f1420;--card:#171e2e;--line:#262f45;--fg:#e8ecf5;--mut:#8b96ad;--ok:#3fb96b;--err:#e05d5d;--off:#7a8398;--warn:#e0a83f}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:720px;margin:0 auto;padding:24px 16px 48px}
h1{font-size:22px;margin:0 0 4px}.sub{color:var(--mut);font-size:13px;margin-bottom:20px}
.banner{border-radius:12px;padding:14px 18px;font-weight:600;margin-bottom:8px}
.b-ok{background:#123322;color:#7be3a8}.b-warn{background:#33270f;color:#f0c878}.b-err{background:#331616;color:#f0a0a0}.b-mut{background:#1c2233;color:var(--mut)}
.note{color:var(--mut);font-size:13px;margin:4px 0 20px}
.card{background:var(--card);border:1px solid var(--line);border-radius:12px;overflow:hidden}
.row{display:flex;align-items:center;gap:12px;padding:12px 16px;border-top:1px solid var(--line)}
.row:first-child{border-top:0}.row .t{flex:1;min-width:0}.row .t b{display:block}.row .t span{color:var(--mut);font-size:12px}
.dot{width:10px;height:10px;border-radius:50%;flex:none}
.d-ok{background:var(--ok)}.d-err{background:var(--err)}.d-off{background:var(--off)}.d-mut{background:var(--warn)}
.badge{font-size:12px;border-radius:99px;padding:2px 10px;white-space:nowrap}
.b-ok2{background:#123322;color:#7be3a8}.b-err2{background:#331616;color:#f0a0a0}.b-off2{background:#222839;color:var(--mut)}.b-mut2{background:#33270f;color:#f0c878}
.up{text-align:right;color:var(--mut);font-size:12px;white-space:nowrap}
.up b{color:var(--fg);font-weight:600}
footer{margin-top:24px;color:var(--mut);font-size:12px}
</style></head><body><main>
<h1>{{if eq .Lang "ru"}}Статус Scilla{{else}}Scilla status{{end}}</h1>
<div class="sub">{{if eq .Lang "ru"}}Доступность серверов и протоколов{{else}}Servers and protocols availability{{end}}</div>
<div class="banner {{.BannerCls}}">{{.Banner}}</div>
<div class="note">{{.Note}}</div>
{{if .Failed}}<div class="note">⚠ {{if eq .Lang "ru"}}нет свежих данных: панель не отвечает аддону{{else}}no fresh data: the panel does not answer the addon{{end}}</div>{{end}}
<div class="card">
{{range .Rows}}<div class="row"><span class="dot d-{{.Status}}"></span>
<div class="t"><b>{{.Title}}</b><span>{{.Node}}</span></div>
<span class="badge {{.Badge}}">{{statusText $.Lang .Status}}</span>
<div class="up"><b>{{.Up24}}</b> 24h<br><b>{{.Up7}}</b> 7d · <b>{{.Up30}}</b> 30d{{if .Since}}<br>{{$.Lang}} {{.Since}}{{end}}</div>
</div>{{end}}
</div>
<footer>{{if eq .Lang "ru"}}Обновлено: {{.Updated}}. Если здесь всё работает, а VPN не подключается — проблема на вашем устройстве или у провайдера: попробуйте другой сервер или протокол, перезапустите приложение.{{else}}Updated: {{.Updated}}. If everything is green here but the VPN does not connect — the problem is on your device or with your provider: try another server or protocol, restart the app.{{end}}</footer>
</main></body></html>`))

func statusText(lang, st string) string {
	ru := lang == "ru"
	switch st {
	case "ok":
		if ru {
			return "Работает"
		}
		return "Up"
	case "error":
		if ru {
			return "Проблемы"
		}
		return "Down"
	case "off":
		if ru {
			return "Выключен"
		}
		return "Off"
	default:
		if ru {
			return "Нет данных"
		}
		return "Unknown"
	}
}

func badgeClass(st string) string {
	switch st {
	case "ok":
		return "b-ok2"
	case "error":
		return "b-err2"
	case "off":
		return "b-off2"
	default:
		return "b-mut2"
	}
}

func (s *server) ui(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ui/index.html" && r.URL.Path != "/ui/" {
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "not_found", "message": "not found"})
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang != "ru" && lang != "en" {
		lang = "en"
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Accept-Language")), "ru") {
			lang = "ru"
		}
	}
	s.mu.Lock()
	st := *s.st
	nodes := append([]panelNode(nil), s.st.Nodes...)
	inbounds := append([]panelInbound(nil), s.st.Inbounds...)
	s.mu.Unlock()
	data := buildPage(lang, &st, nodes, inbounds)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := pageTmpl.Execute(w, data); err != nil {
		s.log.Warn("render", "err", err)
	}
}

func hourKeyOf(t time.Time) string { return t.UTC().Format("2006-01-02T15") }

// uptime sums the hour buckets of one protocol over the last hours.
func uptime(b map[string]*counts, hours int, now time.Time) (pct string, has bool) {
	var ok, bad int
	cut := now.UTC().Add(-time.Duration(hours) * time.Hour)
	for hk, c := range b {
		t, err := time.Parse("2006-01-02T15", hk)
		if err != nil || t.Before(cut.Add(-time.Hour)) {
			continue
		}
		ok += c.OK
		bad += c.Error
	}
	if ok+bad == 0 {
		return "—", false
	}
	return fmt.Sprintf("%d%%", ok*100/(ok+bad)), true
}

func buildPage(lang string, st *state, nodes []panelNode, inbounds []panelInbound) pageData {
	now := time.Now()
	nodeName := map[int64]string{}
	for _, n := range nodes {
		name := n.PublicName
		if name == "" {
			name = n.Name
		}
		nodeName[n.ID] = name
	}
	// Order: by node, then title.
	sort.Slice(inbounds, func(i, j int) bool {
		if inbounds[i].NodeID != inbounds[j].NodeID {
			return inbounds[i].NodeID < inbounds[j].NodeID
		}
		return inbounds[i].Title < inbounds[j].Title
	})
	var rows []row
	errs, oks, offs, unknowns := 0, 0, 0, 0
	oneErr := -1
	for _, in := range inbounds {
		if !in.Enabled {
			offs++
		} else {
			switch in.Status {
			case "ok":
				oks++
			case "error":
				errs++
				if oneErr < 0 {
					oneErr = int(in.ID)
				}
			case "off":
				offs++
			default:
				unknowns++
			}
		}
		b := st.Buckets[fmt.Sprintf("%d", in.ID)]
		u24, _ := uptime(b, 24, now)
		u7, _ := uptime(b, 24*7, now)
		u30, _ := uptime(b, 24*30, now)
		since := ""
		if in.Status == "error" {
			if t, ok := st.Incidents[in.ID]; ok {
				if lang == "ru" {
					since = "с " + t.Local().Format("02.01 15:04")
				} else {
					since = "since " + t.Local().Format("02.01 15:04")
				}
			}
		}
		rows = append(rows, row{Title: in.Title, Node: nodeName[in.NodeID], Status: in.Status, Badge: badgeClass(in.Status), Up24: u24, Up7: u7, Up30: u30, Since: since})
	}
	active := oks + errs + unknowns
	data := pageData{Lang: lang, Rows: rows, Failed: st.PollFailed, Updated: now.Local().Format("02.01.2006 15:04:05")}
	if st.LastPoll.IsZero() {
		data.Checked = "—"
	} else {
		data.Checked = st.LastPoll.Local().Format("15:04:05")
	}
	switch {
	case active == 0 && offs > 0:
		data.Banner, data.BannerCls = pick(lang, "Все протоколы выключены", "Every protocol is off"), "b-mut"
	case errs == 0 && unknowns > 0 && oks == 0:
		data.Banner, data.BannerCls = pick(lang, "Нет данных о серверах", "No data about the servers"), "b-mut"
	case errs == 0 && unknowns > 0:
		data.Banner, data.BannerCls = pick(lang, "Частично нет данных", "Partially no data"), "b-warn"
	case errs == 0:
		data.Banner, data.BannerCls = pick(lang, "Всё работает", "Everything is up"), "b-ok"
	case errs == active:
		data.Banner, data.BannerCls = pick(lang, "Серьёзные проблемы", "Major outage"), "b-err"
	default:
		data.Banner, data.BannerCls = pick(lang, "Частичные проблемы", "Partial outage"), "b-warn"
	}
	data.Note = explain(lang, errs, oks, unknowns, active, oneErr, inbounds)
	return data
}

func pick(lang, ru, en string) string {
	if lang == "ru" {
		return ru
	}
	return en
}

// explain is the answer to "is it the server or not": the scenario picks the words.
func explain(lang string, errs, oks, unknowns, active, oneErr int, inbounds []panelInbound) string {
	ru := lang == "ru"
	switch {
	case errs == 0 && oks > 0:
		if ru {
			return "Все протоколы отвечают. Если у вас не работает VPN — проблема на вашем устройстве или у провайдера."
		}
		return "Every protocol answers. If your VPN does not work — the problem is on your device or with your provider."
	case errs == active:
		if ru {
			return "Недоступны все протоколы: проблема на стороне панели или сети."
		}
		return "Every protocol is down: the problem is on the panel's or the network's side."
	case errs == 1 && oks > 0:
		// One protocol down while the rest answer: most likely blocked by the provider.
		if ru {
			return "Не работает один протокол, остальные отвечают: вероятно, его заблокировал провайдер — попробуйте другой."
		}
		return "One protocol is down while the rest answer: the provider likely blocked it — try another one."
	default:
		if ru {
			return "Часть протоколов недоступна. Проверьте, отвечает ли ваш сервер; если всё зелёное, а VPN не работает — перезапустите приложение или смените сервер."
		}
		return "Some protocols are down. Check whether your server answers; if everything is green but the VPN does not work — restart the app or change the server."
	}
}
