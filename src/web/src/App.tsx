import { Routes, Route } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getEmojiMap } from "./api";
import Header from "./components/Header";
import FinderPage from "./pages/FinderPage";
import DetailPage from "./pages/DetailPage";

export default function App() {
  // Load the slug->emoji map once; cards fall back to 🐾 if it's unavailable.
  const { data: emoji = {} } = useQuery({ queryKey: ["emoji"], queryFn: getEmojiMap });

  return (
    <>
      <div className="bunting" />
      <Header />
      <Routes>
        <Route path="/" element={<FinderPage emoji={emoji} />} />
        <Route path="/animal/:slug" element={<DetailPage emoji={emoji} />} />
      </Routes>
    </>
  );
}
