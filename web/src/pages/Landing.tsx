import { Link } from "react-router-dom";

export default function Landing() {
  return (
    <div className="landing">
      <header className="landing-nav">
        <div className="wordmark">
          Benefit<span className="amp">·</span>Coins
        </div>
        <nav className="landing-nav-links">
          <a href="#how">How it works</a>
          <a href="#features">Features</a>
          <a href="#pricing">Pricing</a>
          <Link to="/login" className="link">
            Sign in
          </Link>
          <Link to="/signup" className="btn-ink">
            Open a household
          </Link>
        </nav>
      </header>

      <section className="hero">
        <p className="kicker">A tiny bank for your household</p>
        <h1>Pocket money, kept like a real ledger.</h1>
        <p className="lede">
          BenefitCoins turns chores into a double-entry account for each kid.
          They earn fractions of a coin for what they do, you approve it, and
          every posting is recorded — no scraps of paper, no arguments about
          who's owed what.
        </p>
        <div className="hero-cta">
          <Link to="/signup" className="btn-ink lg">
            Open your household — free
          </Link>
          <Link to="/login" className="btn-outline lg">
            Sign in
          </Link>
        </div>
        <p className="muted small">
          No card required. You're the operator; your kids are the account
          holders.
        </p>
      </section>

      <section id="how" className="band">
        <h2>How it works</h2>
        <ol className="steps">
          <li>
            <span className="step-no">01</span>
            <h3>Open a household</h3>
            <p>
              Sign up and you become the operator. Your household gets its own
              set of books, sealed off from everyone else's.
            </p>
          </li>
          <li>
            <span className="step-no">02</span>
            <h3>Post chores &amp; approve</h3>
            <p>
              List chores with a coin value. Kids mark them done; each request
              waits as a hold until you approve or decline it.
            </p>
          </li>
          <li>
            <span className="step-no">03</span>
            <h3>Earn, save, redeem</h3>
            <p>
              Fractions add up. At a whole coin, a kid can redeem a reward — the
              spend is deducted and the statement updated.
            </p>
          </li>
        </ol>
      </section>

      <section id="features" className="band alt">
        <h2>Built like a bank core</h2>
        <div className="feature-grid">
          <Feature
            title="Real double-entry ledger"
            body="Every coin is a posting between accounts. Balances can't go negative and nothing double-spends — the ledger enforces it."
          />
          <Feature
            title="Approval holds"
            body="Chores and redemptions post as pending holds. You settle or void them, exactly like an authorization on a card."
          />
          <Feature
            title="Fractional coins"
            body="Take out the trash for 0.15, mow the lawn for 1.00. Amounts are tracked to the milli-coin."
          />
          <Feature
            title="Multiple kids &amp; parents"
            body="Add as many account holders and co-operators as your household needs."
          />
          <Feature
            title="Manual adjustments"
            body="Add a birthday bonus or dock a lost library book, with a reason and date attached to the entry."
          />
          <Feature
            title="Full audit trail"
            body="Every state change is written to an append-only log. See exactly who did what, and when."
          />
        </div>
      </section>

      <section id="pricing" className="band">
        <h2>Pricing</h2>
        <div className="price-grid">
          <div className="price-card">
            <h3>Free</h3>
            <p className="price">
              $0<span>/mo</span>
            </p>
            <ul>
              <li>One household</li>
              <li>Unlimited kids &amp; chores</li>
              <li>Full ledger &amp; audit trail</li>
            </ul>
            <Link to="/signup" className="btn-ink">
              Open a household
            </Link>
          </div>
          <div className="price-card feature">
            <span className="ribbon">Coming soon</span>
            <h3>Family Plus</h3>
            <p className="price">
              $4<span>/mo</span>
            </p>
            <ul>
              <li>Everything in Free</li>
              <li>Recurring allowances</li>
              <li>Savings goals &amp; interest</li>
              <li>Monthly PDF statements</li>
            </ul>
            <button className="btn-outline" disabled>
              Not yet available
            </button>
          </div>
        </div>
      </section>

      <footer className="landing-foot">
        <div className="wordmark small">
          Benefit<span className="amp">·</span>Coins
        </div>
        <p className="muted small">
          A learning project — a household core-banking platform. Play money
          only.
        </p>
      </footer>
    </div>
  );
}

function Feature({ title, body }: { title: string; body: string }) {
  return (
    <div className="feature">
      <h3>{title}</h3>
      <p>{body}</p>
    </div>
  );
}
