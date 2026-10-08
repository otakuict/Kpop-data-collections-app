const shopHues=new Map([
 ['yoichi69',205],['kdatastudio',156],['idollove',278],['idolxdata',46],
 ['datacoffeeshop',24],['loveshakedata',344],['sakuradata',318],['baemonfan',182],
 ['keqingdata',250],['kumdata',94],['kpftime',222],['not sure',58],['no trade',8]
]);

export function sourceHue(source) {
 const name=source.trim().toLowerCase().replace(/\s*\(\?\)$/, '');
 if (!name) return null;
 if (shopHues.has(name)) return shopHues.get(name);
 let hash=0;
 for (const character of name) hash=(Math.imul(hash,31)+character.codePointAt(0))>>>0;
 return hash%360;
}
