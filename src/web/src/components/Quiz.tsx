import { useEffect, useState } from "react";
import { useFinder } from "../store";
import { quiz } from "../quiz";

// Pop-up "Which animal are you?" quiz. Steps through dilemmas; when finished it
// applies the collected traits to the finder filters and closes, so the results
// appear on the page itself (as if you'd set the filters yourself).
export default function Quiz({ onClose }: { onClose: () => void }) {
  const applyProfile = useFinder((s) => s.applyProfile);
  const [answers, setAnswers] = useState<{ include: string[]; exclude: string[] }[]>([]);
  const current = answers.length;
  const finished = current >= quiz.length;

  useEffect(() => {
    if (finished) {
      applyProfile(
        answers.flatMap((a) => a.include),
        answers.flatMap((a) => a.exclude),
      );
      onClose();
    }
  }, [finished]); // eslint-disable-line react-hooks/exhaustive-deps

  if (finished) return null; // applying + closing

  const q = quiz[current];
  const choose = (include: string[] = [], exclude: string[] = []) =>
    setAnswers((a) => [...a, { include, exclude }]);
  const back = () => setAnswers((a) => a.slice(0, -1));

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <button className="modal-close" onClick={onClose} aria-label="close">
          ✕
        </button>

        <div className="quiz-progress">
          {current + 1} / {quiz.length}
        </div>
        <h2 className="quiz-prompt">{q.prompt}</h2>

        <div className="quiz-options">
          {q.options.map((o) => (
            <button key={o.label} className="quiz-option" onClick={() => choose(o.include, o.exclude)}>
              {o.label}
            </button>
          ))}
          <button className="quiz-option quiz-skip" onClick={() => choose()}>
            🤷 Can't decide
          </button>
        </div>

        {current > 0 && (
          <button className="quiz-back" onClick={back}>
            ‹ Back
          </button>
        )}
      </div>
    </div>
  );
}
