import { useEffect, useState, type ReactNode } from "react";
import { useAuth } from "../auth";
import { useBranding } from "../branding";
import { NotificationBell } from "./NotificationBell";
import { enablePushNotifications, isPushEnabled, pushSupported } from "../push";
import { IconBell, IconLogout, IconMenu, IconX } from "./icons";

export interface Section {
  key: string;
  label: string;
  icon: ReactNode;
  render: () => ReactNode;
  badge?: number;
  hint?: string; // optional one-line subtitle under the page title
}

// DashboardShell is the online-banking chrome: a fixed left rail of sections
// on desktop that becomes a slide-out drawer (behind a hamburger toggle) on
// narrow screens, plus a household/user footer and a top bar naming the
// current page.
export function DashboardShell({ sections }: { sections: Section[] }) {
  const { session, logout } = useAuth();
  const brand = useBranding();
  const [activeKey, setActiveKey] = useState(sections[0].key);
  const current = sections.find((s) => s.key === activeKey) ?? sections[0];
  const initial = (session?.username?.[0] ?? "?").toUpperCase();
  const [menuOpen, setMenuOpen] = useState(false);

  // Closing on route change keeps the drawer from staying open after a tap,
  // and body-scroll-lock keeps the page behind it from scrolling.
  const selectSection = (key: string) => {
    setActiveKey(key);
    setMenuOpen(false);
  };
  useEffect(() => {
    document.body.classList.toggle("scroll-lock", menuOpen);
    return () => document.body.classList.remove("scroll-lock");
  }, [menuOpen]);

  const [pushOn, setPushOn] = useState(false);
  useEffect(() => {
    if (pushSupported()) isPushEnabled().then(setPushOn).catch(() => {});
  }, []);
  const handleEnablePush = async () => {
    setPushOn(await enablePushNotifications().catch(() => false));
  };

  return (
    <div className="app">
      {menuOpen && <div className="sidebar-scrim" onClick={() => setMenuOpen(false)} />}
      <aside className={`sidebar ${menuOpen ? "is-open" : ""}`}>
        <div className="brand">
          <span className="brand-mark" aria-hidden="true" />
          <span className="brand-name">{brand.product_name}</span>
          <button className="icon-btn sidebar-close" onClick={() => setMenuOpen(false)} aria-label="Close menu">
            <IconX />
          </button>
        </div>

        <nav className="nav">
          {sections.map((s) => (
            <button
              key={s.key}
              className={`nav-item ${s.key === activeKey ? "is-active" : ""}`}
              onClick={() => selectSection(s.key)}
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
          {pushSupported() && !pushOn && (
            <button className="nav-item push-toggle" onClick={handleEnablePush}>
              <span className="nav-icon"><IconBell /></span>
              <span className="nav-label">Enable push notifications</span>
            </button>
          )}
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
          <button className="icon-btn menu-btn" onClick={() => setMenuOpen(true)} aria-label="Open menu">
            <IconMenu />
          </button>
          <div className="topbar-title">
            <h1 className="page-title">{current.label}</h1>
            {current.hint && <p className="page-hint">{current.hint}</p>}
          </div>
          <div className="topbar-right">
            <NotificationBell />
            <span className="env-chip" title="This is a learning sandbox — no real money moves.">Simulated ledger</span>
          </div>
        </header>
        <div className="content">{current.render()}</div>
      </div>
    </div>
  );
}
