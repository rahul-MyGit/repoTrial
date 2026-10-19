import Link from "next/link";
import { CatalogFrame } from "../../../../components/catalog/Frame";
import styles from "../../../../components/catalog/catalog.module.css";

export default function ProblemNotFound() {
  return (
    <CatalogFrame>
      <div className={styles.state}>
        <h2>Challenge not found.</h2>
        <p>This challenge may have moved or is no longer published.</p>
        <Link href="/problems">Browse challenges</Link>
      </div>
    </CatalogFrame>
  );
}
