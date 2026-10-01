import { Link } from "react-router-dom";
import type { Animal } from "../api";

export default function AnimalCard({ animal, emoji }: { animal: Animal; emoji: string }) {
  return (
    <Link to={`/animal/${animal.slug}`} className="card">
      <div className="patch">{emoji}</div>
      <div className="body">
        <div className="name">{animal.name}</div>
        <div className="mini">
          {animal.traits.slice(0, 3).map((tr) => (
            <span key={tr}>{tr}</span>
          ))}
        </div>
        <p className="preview">{animal.description}</p>
      </div>
      {animal.score ? <div className="match">{Math.round(animal.score * 100)}%</div> : null}
    </Link>
  );
}
