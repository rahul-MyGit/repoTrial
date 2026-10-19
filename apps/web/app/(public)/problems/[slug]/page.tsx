import Link from "next/link";
import { notFound } from "next/navigation";
import { CatalogFrame } from "../../../../components/catalog/Frame";
import { getProblem } from "../../../../components/catalog/api";
import styles from "../../../../components/catalog/catalog.module.css";

type PageProps = { params: Promise<{ slug: string }> };

export const dynamic = "force-dynamic";

export default async function ProblemPage({ params }: PageProps) {
  const { slug } = await params;
  const result = await getProblem(slug);
  if (result.status === "not-found") notFound();

  return (
    <CatalogFrame>
      <Link className={styles.back} href="/problems">← All challenges</Link>
      {result.status === "ready" ? (
        <article className={styles.detail}>
          <span className={styles.eyebrow}>Practice challenge / {result.data.language}</span>
          <h1>{result.data.title}</h1>
          <p className={styles.summary}>{result.data.summary}</p>
          <div className={styles.metadata}>
            <span>{result.data.difficulty}</span>
            <span className={styles.separator}>/</span>
            <span>Version {result.data.version}</span>
            {result.data.tags?.map((tag) => <span className={styles.tag} key={tag}>{tag}</span>)}
          </div>
          <section className={styles.brief} aria-labelledby="brief-title">
            <h2 id="brief-title">Your starting point</h2>
            <p>{result.data.summary}</p>
          </section>
          <div className={styles.notice} role="status">Starting an attempt will be available when the practice workspace opens. You will run the broken service before seeing its source.</div>
        </article>
      ) : (
        <div className={styles.state} role="status">
          <h2>We could not load this challenge.</h2>
          <p>The catalog is temporarily unavailable. Try again in a moment.</p>
          <Link href={`/problems/${encodeURIComponent(slug)}`}>Try again</Link>
        </div>
      )}
    </CatalogFrame>
  );
}
