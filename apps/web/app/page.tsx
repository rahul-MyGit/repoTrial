const steps = [
  {
    number: "01",
    title: "Read the failure",
    description: "Run the broken service and study its output before opening the source.",
  },
  {
    number: "02",
    title: "Make a diagnosis",
    description: "Record what you think went wrong and the evidence behind your guess.",
  },
  {
    number: "03",
    title: "Direct the AI",
    description: "Explore a real repository and review each patch the assistant proposes.",
  },
];

export default function HomePage() {
  return (
    <main>
      <header className="site-header">
        <a className="brand" href="/" aria-label="RepoTrial home">
          <span className="brand-mark" aria-hidden="true">R<span>/</span></span>
          <span>RepoTrial</span>
        </a>
        <span className="header-label">A debugging practice space</span>
      </header>

      <section className="hero" aria-labelledby="hero-title">
        <div className="hero-copy">
          <span className="eyebrow"><span className="status-dot" /> Practice, not a tutorial</span>
          <h1 id="hero-title">Find the bug.<br /><em>Explain the why.</em></h1>
          <p>
            Step into an unfamiliar codebase, reason from a real failure, and guide an AI
            assistant toward a fix. You own the diagnosis and every change.
          </p>
          <div className="hero-note">
            <span className="note-line" aria-hidden="true" />
            The first practice challenge is in development.
          </div>
        </div>
        <div className="workspace-preview" aria-label="Preview of the RepoTrial workspace">
          <div className="preview-bar"><span /><span /><span /><strong>checkout-service</strong></div>
          <div className="preview-body">
            <div className="preview-tree">
              <span className="preview-heading">EXPLORER</span>
              <span>⌄ &nbsp;checkout-service</span>
              <span className="tree-child">⌄ &nbsp;internal</span>
              <span className="tree-child tree-file">service.go</span>
              <span className="tree-child tree-file">repository.go</span>
              <span className="tree-child tree-file">handler.go</span>
              <span className="tree-child">▸ &nbsp;tests</span>
            </div>
            <div className="preview-editor">
              <span className="preview-tab">service.go</span>
              <div className="preview-code" aria-hidden="true">
                <span><i>18</i><b>func</b> (s *Service) Checkout(...)</span>
                <span><i>19</i> &nbsp;existing, err := s.repo.Find(key)</span>
                <span><i>20</i> &nbsp;<b>if</b> err != nil {'{'}</span>
                <span><i>21</i> &nbsp;&nbsp;<b>return</b> s.repo.Create(order)</span>
                <span className="code-highlight"><i>22</i> &nbsp;{'}'} <b>return</b> ErrDuplicateKey</span>
                <span><i>23</i>{'}'}</span>
              </div>
              <div className="preview-terminal">
                <span>RUN OUTPUT</span>
                <code>retry with same key: expected 200, got 409</code>
              </div>
            </div>
            <div className="preview-assistant">
              <span className="preview-heading">AI ASSISTANT</span>
              <div className="assistant-bubble">What does the failing retry tell us about the current flow?</div>
              <div className="assistant-line" /><div className="assistant-line short" />
              <div className="assistant-input">Ask about the codebase <span>↗</span></div>
            </div>
          </div>
        </div>
      </section>

      <section className="process" aria-labelledby="process-title">
        <div className="section-intro"><span className="eyebrow">THE WORKFLOW</span><h2 id="process-title">A real debugging loop.</h2></div>
        <div className="step-grid">
          {steps.map((step) => (
            <article className="step" key={step.number}>
              <span className="step-number">{step.number}</span>
              <h3>{step.title}</h3>
              <p>{step.description}</p>
            </article>
          ))}
        </div>
      </section>

      <footer><span>RepoTrial</span><span>Built for the moments between an error and an answer.</span></footer>
    </main>
  );
}
