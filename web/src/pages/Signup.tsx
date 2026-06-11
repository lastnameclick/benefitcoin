import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { ApiError } from "../api";

export default function Signup() {
  const { signup } = useAuth();
  const navigate = useNavigate();
  const [form, setForm] = useState({ household_name: "", display_name: "", email: "", password: "" });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm({ ...form, [k]: e.target.value });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await signup(form);
      navigate("/app");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create your household.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="center">
      <form className="card login" onSubmit={submit}>
        <div className="masthead-mini">
          <Link to="/" className="wordmark">Benefit<span className="amp">·</span>Coins</Link>
          <div className="eyebrow">Open a household</div>
        </div>
        <div className="form">
          <label>Household name
            <input value={form.household_name} onChange={set("household_name")} placeholder="The Rivera Family" autoFocus />
          </label>
          <label>Your name
            <input value={form.display_name} onChange={set("display_name")} placeholder="Sam Rivera" />
          </label>
          <label>Email
            <input type="email" value={form.email} onChange={set("email")} placeholder="you@example.com" />
          </label>
          <label>Password
            <input type="password" value={form.password} onChange={set("password")} placeholder="At least 6 characters" />
          </label>
          {error && <div className="error">{error}</div>}
          <button className="primary" disabled={busy}>{busy ? "Setting up…" : "Create household"}</button>
        </div>
        <p className="muted small switch">
          Already have an account? <Link to="/login">Sign in</Link>
        </p>
      </form>
    </div>
  );
}
