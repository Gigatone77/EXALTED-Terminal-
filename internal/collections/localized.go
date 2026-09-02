package collections

import "strings"

// localizedOrdinal maps localized book names/abbreviations to their 1-based
// ordinal within a collection. It lets references like "Juan 3:16",
// "Gênesis 1:1", or "Hechos" resolve correctly for non-English Bibles.
// Keyed by collection, lowercased and stripped of spaces.
func localizedOrdinal(collectionID, s string) (int, bool) {
	if collectionID == "" {
		collectionID = "bible"
	}
	key := normalize(s)
	if key == "" {
		return 0, false
	}
	// build the per-language map lazily.
	table := localizedTables[collectionID]
	if table == nil {
		table = buildLocalizedTable()
		localizedTables[collectionID] = table
	}
	if ord, ok := table[key]; ok {
		return ord, true
	}
	return 0, false
}

var localizedTables = map[string]map[string]int{}

func buildLocalizedTable() map[string]int {
	m := map[string]int{}
	add := func(names string, ord int) {
		for _, n := range strings.Split(names, ",") {
			k := normalize(n)
			if k != "" {
				if _, exists := m[k]; !exists {
					m[k] = ord
				}
			}
		}
	}
	// Spanish & Portuguese (and select others) book names.
	add("Genesis,Gênesis,Génesis,Gn", 1)
	add("Exodo,Êxodo,Éxodo,Ex", 2)
	add("LevIt,Levitico,Levitico,Lev", 3)
	add("Numeros,Números,Números", 4)
	add("Deuteronomio,Dt", 5)
	add("Josue,Josué,Jos", 6)
	add("Jueces,Jz,Is", 7)
	add("Rut,Rt", 8)
	add("1Samuel,1Sam,1Sm", 9)
	add("2Samuel,2Sam,2Sm", 10)
	add("1Reyes,1Reis,1Rs", 11)
	add("2Reyes,2Reis,2Rs", 12)
	add("1Cronicas,1Crônicas,1Crôn", 13)
	add("2Cronicas,2Crônicas,2Crôn", 14)
	add("Esdras,Esd", 15)
	add("Nehemias,Neemias,Ne", 16)
	add("Ester,Et", 17)
	add("Job,Jó,Jb", 18)
	add("Salmos,Sal,Sl", 19)
	add("Proverbios,Prov,Pr", 20)
	add("Eclesiastes,Ecl,Co", 21)
	add("Cantares,Cantico de los Cantares,Cânticos,Ct", 22)
	add("Isaias,Isaías,Is", 23)
	add("Jeremias,Jeremías,Jer", 24)
	add("Lamentaciones,Lm", 25)
	add("Ezequiel,Ez", 26)
	add("Daniel,Dn,Da", 27)
	add("Oseas,Oseias,Os", 28)
	add("Joel,Jl", 29)
	add("Amos,Amós,Am", 30)
	add("Abdias,Obadias,Obadias,Ab", 31)
	add("Jonas,Jonas,Jn", 32)
	add("Miqueas,Mq, Miquéias", 33)
	add("Nahum,Naum,Nah", 34)
	add("Habacuc,Hab", 35)
	add("Sofonias,Sofonias,So,Sof", 36)
	add("Ageo,Ageu,Ag", 37)
	add("Zacarias,Zacarias,Zc", 38)
	add("Malaquias,Malaquias,Ml", 39)
	add("Mateo,Mateus,Mat,Mt", 40)
	add("Marcos,Mc", 41)
	add("Lucas,Lc", 42)
	add("Juan,Joao,João,Jn,Jo", 43)
	add("Hechos,Atos,Act", 44)
	add("Romanos,Rm,Ro", 45)
	add("1Corintios,1Coríntios,1Co", 46)
	add("2Corintios,2Coríntios,2Co", 47)
	add("Galatas,Galatas,Gl", 48)
	add("Efesios,Ef", 49)
	add("Filipenses,Fp,Fil", 50)
	add("Colosenses,Cl,Col", 51)
	add("1Tesalonicenses,1Tessalonicenses,1Ts", 52)
	add("2Tesalonicenses,2Tessalonicenses,2Ts", 53)
	add("1Timoteo,1Tm", 54)
	add("2Timoteo,2Tm", 55)
	add("Tito,Tt", 56)
	add("Filemon,Filemón,Fm", 57)
	add("Hebreos,Hb", 58)
	add("Santiago,Tg", 59)
	add("1Pedro,1Pe,1Pd", 60)
	add("2Pedro,2Pe,2Pd", 61)
	add("1Juan,1João,1Jo", 62)
	add("2Juan,2João,2Jo", 63)
	add("3Juan,3João,3Jo", 64)
	add("Judas,Jd,Jds", 65)
	add("Apocalipsis,Apocalipse,Ap", 66)
	return m
}
