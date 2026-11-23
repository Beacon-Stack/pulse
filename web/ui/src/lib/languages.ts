// Display names and flags for catalog language codes.

const LANG_DATA: Record<string, { flag: string; label: string }> = {
  "en-US": { flag: "\u{1F1FA}\u{1F1F8}", label: "English" },
  "en-AU": { flag: "\u{1F1E6}\u{1F1FA}", label: "English (AU)" },
  "en-GB": { flag: "\u{1F1EC}\u{1F1E7}", label: "English (UK)" },
  "fr-FR": { flag: "\u{1F1EB}\u{1F1F7}", label: "French" },
  "es-ES": { flag: "\u{1F1EA}\u{1F1F8}", label: "Spanish" },
  "de-DE": { flag: "\u{1F1E9}\u{1F1EA}", label: "German" },
  "it-IT": { flag: "\u{1F1EE}\u{1F1F9}", label: "Italian" },
  "pt-BR": { flag: "\u{1F1E7}\u{1F1F7}", label: "Portuguese" },
  "pt-PT": { flag: "\u{1F1F5}\u{1F1F9}", label: "Portuguese (PT)" },
  "ru-RU": { flag: "\u{1F1F7}\u{1F1FA}", label: "Russian" },
  "uk-UA": { flag: "\u{1F1FA}\u{1F1E6}", label: "Ukrainian" },
  "hu-HU": { flag: "\u{1F1ED}\u{1F1FA}", label: "Hungarian" },
  "ro-RO": { flag: "\u{1F1F7}\u{1F1F4}", label: "Romanian" },
  "pl-PL": { flag: "\u{1F1F5}\u{1F1F1}", label: "Polish" },
  "ja-JP": { flag: "\u{1F1EF}\u{1F1F5}", label: "Japanese" },
  "zh-CN": { flag: "\u{1F1E8}\u{1F1F3}", label: "Chinese" },
  "zh-TW": { flag: "\u{1F1F9}\u{1F1FC}", label: "Chinese (TW)" },
  "ko-KR": { flag: "\u{1F1F0}\u{1F1F7}", label: "Korean" },
  "da-DK": { flag: "\u{1F1E9}\u{1F1F0}", label: "Danish" },
  "sv-SE": { flag: "\u{1F1F8}\u{1F1EA}", label: "Swedish" },
  "nl-NL": { flag: "\u{1F1F3}\u{1F1F1}", label: "Dutch" },
  "fi-FI": { flag: "\u{1F1EB}\u{1F1EE}", label: "Finnish" },
  "cs-CZ": { flag: "\u{1F1E8}\u{1F1FF}", label: "Czech" },
  "tr-TR": { flag: "\u{1F1F9}\u{1F1F7}", label: "Turkish" },
  "ar-SA": { flag: "\u{1F1F8}\u{1F1E6}", label: "Arabic" },
  "he-IL": { flag: "\u{1F1EE}\u{1F1F1}", label: "Hebrew" },
  "bg-BG": { flag: "\u{1F1E7}\u{1F1EC}", label: "Bulgarian" },
  "th-TH": { flag: "\u{1F1F9}\u{1F1ED}", label: "Thai" },
  "vi-VN": { flag: "\u{1F1FB}\u{1F1F3}", label: "Vietnamese" },
  "sk-SK": { flag: "\u{1F1F8}\u{1F1F0}", label: "Slovak" },
  "hr-HR": { flag: "\u{1F1ED}\u{1F1F7}", label: "Croatian" },
  "el-GR": { flag: "\u{1F1EC}\u{1F1F7}", label: "Greek" },
  "fa-IR": { flag: "\u{1F1EE}\u{1F1F7}", label: "Persian" },
};

export function langDisplay(code: string): { flag: string; label: string } {
  return LANG_DATA[code] ?? { flag: "\u{1F310}", label: code };
}
