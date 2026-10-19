import { CatalogFrame } from "../../../components/catalog/Frame";
import styles from "../../../components/catalog/catalog.module.css";

export default function ProblemsLoading() {
  return (
    <CatalogFrame>
      <div className={styles.state} role="status" aria-live="polite">
        <h2>Loading challenges…</h2>
        <p>Checking the published problem catalog.</p>
      </div>
    </CatalogFrame>
  );
}
