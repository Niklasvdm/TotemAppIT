import i18n from "i18next";
import { initReactI18next } from "react-i18next";

// UI chrome strings only — the animal CONTENT (names/descriptions/traits) is
// already language-projected by the API via ?lang=.
const resources = {
  it: {
    translation: {
      brand: "Cercatore di Totem",
      tagline: "Scouts en Gidsen · Totemzoeker",
      searchAnimal: "cerca un animale… es. lupo, aquila",
      modeExact: "🔍 Esatto",
      modeSimilar: "✨ Simile",
      included: "✓ Incluse",
      excluded: "✗ Escluse",
      traits: "🏷️ Caratteristiche",
      removeAll: "Rimuovi tutto",
      searchTrait: "🔎 Cerca una caratteristica…",
      none: "Nessuna",
      noResults: "Nessun risultato",
      found: "{{n}} totem trovati",
      similarTo: "✨ Totem simili",
      back: "‹ Tutti i totem",
      loading: "Caricamento…",
    },
  },
  en: {
    translation: {
      brand: "Totem Finder",
      tagline: "Scouts en Gidsen · Totem finder",
      searchAnimal: "search an animal… e.g. wolf, eagle",
      modeExact: "🔍 Exact",
      modeSimilar: "✨ Similar",
      included: "✓ Included",
      excluded: "✗ Excluded",
      traits: "🏷️ Traits",
      removeAll: "Remove all",
      searchTrait: "🔎 Search a trait…",
      none: "None",
      noResults: "No results",
      found: "{{n}} totems found",
      similarTo: "✨ Similar totems",
      back: "‹ All totems",
      loading: "Loading…",
    },
  },
  nl: {
    translation: {
      brand: "Totemzoeker",
      tagline: "Scouts en Gidsen · Totemzoeker",
      searchAnimal: "zoek een dier… bv. wolf, arend",
      modeExact: "🔍 Exact",
      modeSimilar: "✨ Gelijkaardig",
      included: "✓ Inbegrepen",
      excluded: "✗ Uitgesloten",
      traits: "🏷️ Kenmerken",
      removeAll: "Alles wissen",
      searchTrait: "🔎 Zoek een kenmerk…",
      none: "Geen",
      noResults: "Geen resultaten",
      found: "{{n}} totems gevonden",
      similarTo: "✨ Gelijkaardige totems",
      back: "‹ Alle totems",
      loading: "Laden…",
    },
  },
};

i18n.use(initReactI18next).init({
  resources,
  lng: "en",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});

export default i18n;
