import Link from "next/link";
import { CatalogFrame } from "../../../components/catalog/Frame";
import { listProblems } from "../../../components/catalog/api";
import styles from "../../../components/catalog/catalog.module.css";

type PageProps = { searchParams: Promise<{ cursor?: string }> };

export const dynamic = "force-dynamic";

export default async function ProblemsPage({ searchParams }: PageProps) {
  const { cursor } = await searchParams;
  const result = await listProblems(cursor);

  return (
    <CatalogFrame>
      <div className={styles.intro}>
        <div>
          <span className={styles.eyebrow}>Practice challenges</span>
          <h1>Choose a codebase.</h1>
          <p>Each challenge begins with a failing run. Form a diagnosis, inspect the repository, and guide AI toward a fix.</p>
        </div>
        {result.status === "ready" ? <span className={styles.count}>{result.data.items.length} on this page</span> : null}
      </div>

      {result.status === "ready" && result.data.items.length > 0 ? (
        <>
          <div className={styles.grid}>
            {result.data.items.map((problem) => (
              <Link className={styles.card} href={`/problems/${encodeURIComponent(problem.slug)}`} key={`${problem.slug}:${problem.version}`}>
                <span className={styles.cardTop}><span>{problem.language}</span><span className={styles.separator}>/</span><span>{problem.difficulty}</span></span>
                <h2>{problem.title}</h2>
                <p>{problem.summary}</p>
                <span className={styles.cardFoot}><span>Version {problem.version}</span><span className={styles.arrow} aria-hidden="true">↗</span></span>
              </Link>
            ))}
          </div>
          <nav className={styles.pager} aria-label="Problem pages">
            {cursor ? <Link href="/problems">First page</Link> : <span />}
            {result.data.nextCursor ? (
              <Link href={`/problems?${new URLSearchParams({ cursor: result.data.nextCursor })}`}>Next page →</Link>
            ) : null}
          </nav>
        </>
      ) : result.status === "ready" ? (
        <div className={styles.state} role="status">
          <h2>No published challenges yet.</h2>
          <p>Challenges will appear here as soon as they are ready to practice.</p>
          {cursor ? <Link href="/problems">Back to the first page</Link> : null}
        </div>
      ) : (
        <div className={styles.state} role="status">
          <h2>We could not load the challenges.</h2>
          <p>The catalog is temporarily unavailable. Try again in a moment.</p>
          <Link href="/problems">Try again</Link>
        </div>
      )}
    </CatalogFrame>
  );
}
