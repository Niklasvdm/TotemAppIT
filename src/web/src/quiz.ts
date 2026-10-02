// "Which animal are you?" question set.
// Two kinds, interleaved:
//   • dilemmas — a genuine tradeoff; each option INCLUDES one trait.
//   • flaw self-reads — "That's me / Not me"; "That's me" EXCLUDES the virtue,
//     "Not me" INCLUDES it (the "you are not" signals).
// Each option maps to real Dutch trait keys. The UI adds a "Can't decide" that
// changes nothing. Conflicts (a trait both included and excluded) resolve to
// include-wins in the store.
// TODO: translate prompts/options to it/nl; move to data/quiz.json + /api/v1/quiz.

export interface QuizOption {
  label: string;
  include?: string[];
  exclude?: string[];
}
export interface QuizQuestion {
  prompt: string;
  options: QuizOption[];
}

export const quiz: QuizQuestion[] = [
  {
    prompt:
      "You have a deadline tomorrow that would land you a promotion — and tonight a close friend urgently needs you to talk through a personal crisis.",
    options: [
      { label: "Drop the work and be there for them", include: ["zorgzaam"] },
      { label: "Finish the work, call them afterwards", include: ["doelgericht"] },
    ],
  },
  {
    prompt: "After a draining week, your whole crew wants you out for a big night.",
    options: [
      { label: "Go — people are how you recharge", include: ["sociaal"] },
      { label: "Stay in and recharge on your own", include: ["solitair"] },
    ],
  },
  {
    prompt: "I lose interest the moment the novelty wears off.",
    options: [
      { label: "That's me", exclude: ["volhardend"] },
      { label: "Not me", include: ["volhardend"] },
    ],
  },
  {
    prompt:
      "A rare opportunity shows up, but you'd have to commit right now — no time to look into it.",
    options: [
      { label: "Jump on it", include: ["snel"] },
      { label: "Hold back until you understand it", include: ["voorzichtig"] },
    ],
  },
  {
    prompt:
      "You can take the route you know, or an unknown detour that might be amazing — or a total waste of time.",
    options: [
      { label: "Take the detour", include: ["nieuwsgierig"] },
      { label: "Stick with what you know", include: ["honkvast"] },
    ],
  },
  {
    prompt: "Sitting still doing nothing for a whole day would drive me mad.",
    options: [
      { label: "That's me", exclude: ["rustig"] },
      { label: "Not me", include: ["rustig"] },
    ],
  },
  {
    prompt: "Halfway in, it's clear your plan isn't working.",
    options: [
      { label: "Change course", include: ["aanpassend"] },
      { label: "Push through anyway", include: ["volhardend"] },
    ],
  },
  {
    prompt: "You're up against something far bigger than you.",
    options: [
      { label: "Meet it head-on", include: ["sterk"] },
      { label: "Find a way to outsmart it", include: ["intelligent"] },
    ],
  },
  {
    prompt: "New places and strangers make me want to retreat.",
    options: [
      { label: "That's me", exclude: ["sociaal"] },
      { label: "Not me", include: ["sociaal"] },
    ],
  },
  {
    prompt: "Someone new wants into your inner circle, fast.",
    options: [
      { label: "Stay guarded until they prove themselves", include: ["waakzaam"] },
      { label: "Welcome them and look out for them", include: ["beschermend"] },
    ],
  },
  {
    prompt: "Something you want badly will take a long, slow grind to reach.",
    options: [
      { label: "Wait it out, however long it takes", include: ["geduldig"] },
      { label: "Hustle to find a shortcut", include: ["vindingrijk"] },
    ],
  },
  {
    prompt: "I'd rather wing it than plan ahead.",
    options: [
      { label: "That's me", exclude: ["voorzichtig"] },
      { label: "Not me", include: ["voorzichtig"] },
    ],
  },
  {
    prompt: "An afternoon suddenly frees up.",
    options: [
      { label: "Fill it — there's always something to do", include: ["actief"] },
      { label: "Do nothing and enjoy the quiet", include: ["rustig"] },
    ],
  },
  {
    prompt: "Life throws a tricky obstacle in your path.",
    options: [
      { label: "Power straight through it", include: ["krachtig"] },
      { label: "Slip around it, light on your feet", include: ["wendbaar"] },
    ],
  },
];
