// Rules + a worked example for each game, shown behind the ⓘ button on the games
// index and in-game. English for now (the game UIs are English too); i18n TODO.
// Keys are the client slugs used in GamesPage / the routes.

export interface GameRules {
  icon: string;
  title: string;
  players: string;
  summary: string;
  steps: string[];
  example: string;
}

export const GAME_RULES: Record<string, GameRules> = {
  bomberman: {
    icon: "💣",
    title: "Bomberman",
    players: "2–4 players",
    summary: "Blow up crates and opponents — last one standing wins.",
    steps: [
      "Move with WASD / the arrow keys, or the on-screen pad on mobile.",
      "Drop a bomb with Space (or the 💣 button); it explodes in a + shape a moment later.",
      "Blast crates to reveal power-ups: more bombs, a bigger blast, extra speed.",
      "Never stand in your own blast. The last player alive wins the round.",
    ],
    example: "Drop a bomb beside a crate, then duck around a corner before it goes off.",
  },
  codenames: {
    icon: "🕵️",
    title: "Codenames",
    players: "4+ players, two teams",
    summary: "Spymasters give one-word clues; their team finds its agents without hitting the assassin.",
    steps: [
      "Two teams. One player per team is the Spymaster and alone sees the secret colour key.",
      "On your team's turn the Spymaster says one word + a number — how many cards the word points to.",
      "Teammates tap cards. Your colour → keep guessing; a bystander or the enemy's card → your turn ends.",
      "Tap the single assassin and your team loses instantly. First team to find all its agents wins.",
    ],
    example: "Your agents include APPLE, BANANA and GRAPE → clue: “Fruit, 3”.",
  },
  themind: {
    icon: "🧠",
    title: "The Mind",
    players: "2–4 players, co-op",
    summary: "Play your cards onto one pile in rising order — with no talking.",
    steps: [
      "Everyone holds number cards from 1–100 (one each at level 1, two at level 2, and so on).",
      "At any moment, anyone may play their lowest card onto the shared pile.",
      "No talking about numbers — read the timing together.",
      "Play while someone still holds a lower card and you lose a life. Clear every level to win.",
    ],
    example: "You hold 48. If it feels like no one has anything lower, play it — but if someone held 12, that's a life lost.",
  },
  thegame: {
    icon: "🃏",
    title: "The Game",
    players: "1–5 players, co-op",
    summary: "Empty the whole deck onto four piles — two count up, two count down.",
    steps: [
      "Two piles start at 1 and must go UP; two start at 100 and must go DOWN.",
      "On your turn play at least 2 cards (just 1 once the draw pile is empty).",
      "Up-piles take a higher card; down-piles take a lower one.",
      "The trick: you may also jump exactly 10 the “wrong” way (e.g. an up-pile on 55 accepts 45).",
      "Play every card to win. If a player can't make their minimum, everyone loses.",
    ],
    example: "An up-pile shows 50 → play 51 or 73… or exactly 40 (ten back).",
  },
  wavelength: {
    icon: "📡",
    title: "Wavelength",
    players: "4+ players, two teams",
    summary: "A psychic hints at a hidden spot on a spectrum; their team dials to it.",
    steps: [
      "Two teams. Each round one player on the active team is the Psychic and secretly sees a target zone on a dial between two opposite ideas.",
      "The Psychic gives a one-line clue pointing at the target.",
      "Their team moves the dial to guess — a bullseye is 4 points, then 3, then 2 as you drift further.",
      "The other team then bets whether the real target is left or right of that guess (+1 if right).",
      "First team to 10 points wins.",
    ],
    example: "Spectrum “Cold ↔ Hot”, target near the hot end → clue: “a fresh espresso”.",
  },
  justone: {
    icon: "💡",
    title: "Just One",
    players: "3–8 players, co-op",
    summary: "Help one player guess a mystery word — but matching clues cancel each other out.",
    steps: [
      "Each round one player is the guesser and does not see the mystery word; everyone else does.",
      "Every other player secretly writes a single-word clue for it.",
      "Clues are revealed together — any that are identical (or equal to the word) are struck out.",
      "The guesser reads the surviving clues and makes one guess. Score a point for the whole team if it's right.",
    ],
    example: "Word “Beaver”. Clues “dam”, “wood”, “teeth” → the guesser sees all three. But if two people both wrote “dam”, both vanish.",
  },
  loveletter: {
    icon: "💌",
    title: "Love Letter",
    players: "2–4 players",
    summary: "Hold one secret card, use its power to knock out rivals, and be the last one standing.",
    steps: [
      "You always hold one secret card (values 1–8). On your turn draw a second and play one of the two.",
      "Each card does something: the Guard (1) guesses a rival's card to knock them out, the Baron (3) compares hands, the Prince (5) forces a discard, the Princess (8) loses if you ever discard it.",
      "The Handmaid (4) protects you for a turn; the Countess (7) must be played if you also hold a King or Prince.",
      "Last player left — or the highest card when the deck runs out — wins the round and a token. First to the token goal wins.",
    ],
    example: "You play a Guard and guess “Priest” on Sam. If Sam holds the Priest, they're out of the round.",
  },
  decrypto: {
    icon: "🔐",
    title: "Decrypto",
    players: "4+ players, two teams",
    summary: "Clue your own team toward a secret code while the enemy tries to crack it from your clues.",
    steps: [
      "Two teams, each sharing four secret words in slots 1–4 (only your team sees them).",
      "Each round one teammate sees a 3-digit code (e.g. 4-2-1) and gives one clue per digit, hinting at the word in that slot.",
      "Your team must decode its own code; the other team tries to intercept it from the clues (and all past clues).",
      "Intercept the enemy twice to win. Miss your own code twice and you lose.",
    ],
    example: "Your words are 1 Apple, 2 River, 3 Clock, 4 Tiger. Code 3-1-4 → clues “tick”, “core”, “stripes”.",
  },
  hanabi: {
    icon: "🎆",
    title: "Hanabi",
    players: "2–5 players, co-op",
    summary: "Build five firework stacks 1→5 — but you can see everyone's cards except your own.",
    steps: [
      "You hold your cards facing OUT: everyone sees them but you. You only know what others have hinted.",
      "On your turn do one thing: give a hint (spend a token, point out all of one colour or one number in a teammate's hand), discard a card (regain a token), or play a card.",
      "A played card must be the next number for its colour (stacks go 1→5). A wrong play burns one of three fuses.",
      "Lose all three fuses and the show ends. Finish all five stacks for a perfect 25.",
    ],
    example: "A teammate's hand shows a red 1 (you can see it, they can't). Hint “red”, or hint “1”, to point it out.",
  },
};
