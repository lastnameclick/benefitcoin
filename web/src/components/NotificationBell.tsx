import { useEffect, useRef, useState } from "react";
import { useNotifications } from "../notifications";
import { relativeTime } from "../lib/time";
import { IconBell } from "./icons";

// NotificationBell is the topbar affordance for the in-app feed: a badge with
// the unread count, and a dropdown of recent notifications. Live updates
// arrive via NotificationsProvider's SSE connection — no polling here.
export function NotificationBell() {
  const { notifications, unreadCount, markRead, markAllRead } = useNotifications();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  return (
    <div className="notif-bell" ref={rootRef}>
      <button
        className="icon-btn"
        aria-label={unreadCount ? `${unreadCount} unread notifications` : "Notifications"}
        onClick={() => setOpen((v) => !v)}
      >
        <IconBell />
        {unreadCount > 0 && <span className="notif-badge">{unreadCount > 9 ? "9+" : unreadCount}</span>}
      </button>

      {open && (
        <div className="notif-dropdown">
          <div className="notif-dropdown-head">
            <span>Notifications</span>
            {unreadCount > 0 && (
              <button className="notif-mark-all" onClick={markAllRead}>
                Mark all read
              </button>
            )}
          </div>
          {notifications.length === 0 ? (
            <div className="notif-empty">Nothing yet — you'll see redemption, chore, and bounty updates here.</div>
          ) : (
            <ul className="notif-list">
              {notifications.slice(0, 20).map((n) => (
                <li
                  key={n.id}
                  className={`notif-item ${n.read_at ? "" : "is-unread"}`}
                  onClick={() => !n.read_at && markRead(n.id)}
                >
                  <div className="notif-item-title">{n.title}</div>
                  <div className="notif-item-body">{n.body}</div>
                  <div className="notif-item-time">{relativeTime(n.created_at)}</div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
