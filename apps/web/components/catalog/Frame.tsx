import Link from "next/link";
import styles from "./catalog.module.css";

export function CatalogFrame({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className={styles.shell}>
      <header className={styles.header}>
        <Link className={styles.brand} href="/" aria-label="RepoTrial home">
          <span className={styles.mark} aria-hidden="true">R/</span>
          <span>RepoTrial</span>
        </Link>
        <nav className={styles.nav} aria-label="Main navigation">
          <Link href="/">Home</Link>
          <Link href="/problems" aria-current="page">Problems</Link>
        </nav>
      </header>
      <main className={styles.main}>{children}</main>
    </div>
  );
}
