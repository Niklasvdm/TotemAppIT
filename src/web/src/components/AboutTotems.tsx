// "How totems really work" — a homage to the real Flemish scouting tradition.
// English for now (TODO: it/nl). Mirrors the README's tradition section.
export default function AboutTotems({ onClose }: { onClose: () => void }) {
  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal about" onClick={(e) => e.stopPropagation()}>
        <button className="modal-close" onClick={onClose} aria-label="close">
          ✕
        </button>

        <h2 className="quiz-prompt">🐾 How totems really work</h2>

        <img className="about-photo" src="/campfire.jpg" alt="Friends gathered around a campfire at night" />
        <p className="about-credit">Photo: Elias Strale / Pexels</p>

        <p>
          In Belgian and Flemish scouting, your <strong>totem</strong> is an animal name your group
          gives you. It is chosen to match your <strong>character</strong>, never your looks, and it
          ends up stitched onto your <strong>uniform</strong>. You carry it for life.
        </p>
        <p>
          You <strong>earn</strong> it by completing a <em>proef</em> (a challenge) first. These vary
          a lot per group, but they are about growth and testing your limits. Think of a solo
          overnight or a long trek, a day without speaking, or an endurance or creative task.
        </p>
        <p>
          To pick which animal, the group leafs through a <strong>totemboek</strong>, a book of
          animals and their traits, and chooses the one that fits you best.{" "}
          <em>This app is a digital totemboek.</em> Later you may also get a{" "}
          <strong>voortotem</strong>, an adjective for a standout trait (for example{" "}
          <em>Playful Dolphin</em> or <em>Cheerful Coati</em>).
        </p>
        <p>
          It is revealed at a <strong>campfire</strong>, often at night. The new name is announced to
          the circle, sometimes shouted to the four winds while the group whispers it back.
        </p>
        <p className="about-quote">‘Voor ons ben jij een…’ (“To us, you are a…”)</p>

        <p className="about-origin">
          I built this because I loved this tradition and wanted to do the same with my friends in
          another country, so I translated it and built on top of it.
          <span className="about-sign">Veelzijdige Bever</span>
        </p>

        <p className="about-note">
          This app is for inspiration and fun. A real totem is given by your group, not by an app.{" "}
          <a
            href="https://www.scoutsengidsenvlaanderen.be/scouts-en-gidsenleden/activiteiten/rituelen-en-totems/totemisatie"
            target="_blank"
            rel="noreferrer"
          >
            Learn more at Scouts en Gidsen Vlaanderen ↗
          </a>
        </p>
      </div>
    </div>
  );
}
