import { useState } from "react";
import { Routes, Route } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getEmojiMap } from "./api";
import Header from "./components/Header";
import FinderPage from "./pages/FinderPage";
import DetailPage from "./pages/DetailPage";
import Quiz from "./components/Quiz";
import AboutTotems from "./components/AboutTotems";
import SuggestModal from "./components/SuggestModal";
import GamesPage from "./games/GamesPage";
import BombermanPage from "./games/bomberman/BombermanPage";
import CodenamesPage from "./games/codenames/CodenamesPage";
import MindPage from "./games/themind/MindPage";

export default function App() {
  // Load the slug->emoji map once; cards fall back to 🐾 if it's unavailable.
  const { data: emoji = {} } = useQuery({ queryKey: ["emoji"], queryFn: getEmojiMap });
  const [quizOpen, setQuizOpen] = useState(false);
  const [suggestOpen, setSuggestOpen] = useState(false);
  // Auto-show the info pop-up on first visit; remember once it's been closed.
  const [aboutOpen, setAboutOpen] = useState(() => {
    try {
      return !localStorage.getItem("totem-about-seen");
    } catch {
      return false;
    }
  });
  const closeAbout = () => {
    try {
      localStorage.setItem("totem-about-seen", "1");
    } catch {
      /* private mode — just close */
    }
    setAboutOpen(false);
  };

  return (
    <>
      <div className="bunting" />
      <Header
        onQuiz={() => setQuizOpen(true)}
        onAbout={() => setAboutOpen(true)}
        onSuggest={() => setSuggestOpen(true)}
      />
      <Routes>
        <Route path="/" element={<FinderPage emoji={emoji} />} />
        <Route path="/animal/:slug" element={<DetailPage emoji={emoji} />} />
        <Route path="/games" element={<GamesPage />} />
        <Route path="/games/bomberman" element={<BombermanPage emoji={emoji} />} />
        <Route path="/games/codenames" element={<CodenamesPage />} />
        <Route path="/games/themind" element={<MindPage />} />
      </Routes>
      {quizOpen && <Quiz onClose={() => setQuizOpen(false)} />}
      {aboutOpen && <AboutTotems onClose={closeAbout} />}
      {suggestOpen && <SuggestModal onClose={() => setSuggestOpen(false)} />}
    </>
  );
}
