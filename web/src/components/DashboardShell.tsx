import { useState, type ReactNode } from "react";
import { useAuth } from "../auth";
import { useBranding } from "../branding";
import { IconLogout } from "./icons";

export interface Section {
  key: string;
  label: string;
  icon: ReactNode;
  render: () => ReactNode;
  badge?: number;
  hint?: string; // optional one-line subtitle under the page title
}

// DashboardShell is the online-banking chrome: a fixed left rail of sections, a
// household/user footer, and a top bar naming the current page.
export function DashboardShell({ sections }: { sections: Section[] }) {
  const { session, logout } = useAuth();
  const brand = useBranding();
  const [activeKey, setActiveKey] = useState(sections[0].key);
  const current = sections.find((s) => s.key === activeKey) ?? sections[0];
  const initial = (session?.username?.[0] ?? "?").toUpperCase();

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true" />
          <span className="brand-name">{brand.product_name}</span>
        </div>

        <nav className="nav">
          {sections.map((s) => (
            <button
              key={s.key}
              className={`nav-item ${s.key === activeKey ? "is-active" : ""}`}
              onClick={() => setActiveKey(s.key)}
            >
              <span className="nav-icon">{s.icon}</span>
              <span className="nav-label">{s.label}</span>
              {s.badge ? <span className="nav-badge">{s.badge}</span> : null}
            </button>
          ))}
        </nav>

        <div className="side-foot">
          <div className="house">
            <div className="house-label">Household</div>
            <div className="house-name">{session?.household || "—"}</div>
          </div>
          <div className="user">
            <span className="avatar">{initial}</span>
            <div className="user-meta">
              <div className="user-name">{session?.username}</div>
              <div className="user-role">{session?.role}</div>
            </div>
            <button className="icon-btn" onClick={logout} aria-label="Sign out"><IconLogout /></button>
          </div>
        </div>
      </aside>

      <div className="main">
        <header className="topbar">
          <div>
            <h1 className="page-title">{current.label}</h1>
            {current.hint && <p className="page-hint">{current.hint}</p>}
          </div>
          <div className="topbar-right">
            <span className="env-chip" title="This is a learning sandbox — no real money moves.">Simulated ledger</span>
          </div>
        </header>
        <div className="content">{current.render()}</div>
      </div>
    </div>
  );
}
