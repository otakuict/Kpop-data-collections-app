// Parse a copied Bash cURL command as text. Nothing from the command is run.
function shellWords(input){
 const words=[];
 let word='',quote='',started=false;
 const source=input.replace(/(^|\n)&#x20;/g,'$1 ');
 for(let i=0;i<source.length;i++){
  const char=source[i];
  if(quote==="'"){
   if(char==="'")quote='';else word+=char;
  }else if(quote==='"'){
   if(char==='"')quote='';
   else if(char==='\\'&&i+1<source.length){
    const next=source[i+1];
    if(next==='\n'){i++;}else if('$`"\\'.includes(next)){word+=next;i++;}else word+=char;
   }else word+=char;
  }else if(char==="'"||char==='"'){
   quote=char;started=true;
  }else if(char==='\\'&&i+1<source.length){
   const next=source[++i];
   if(next==='\r'&&source[i+1]==='\n')i++;
   else if(next!=='\n')word+=next;
   started=true;
  }else if(/\s/.test(char)){
   if(started){words.push(word);word='';started=false;}
  }else{word+=char;started=true;}
 }
 if(quote)throw new Error('The cURL command has an unfinished quote.');
 if(started)words.push(word);
 return words;
}

export function parseTeraBoxCurl(input){
 const words=shellWords(input.trim());
 if(!['curl','curl.exe'].includes(words[0]))throw new Error('Paste a Bash cURL command copied from the TeraBox /api/list request.');
 let requestURL='',cookie='',headerCookie='';
 for(let i=1;i<words.length;i++){
  const word=words[i];
  if(word==='--url'){requestURL=words[++i]||'';}
  else if(word.startsWith('--url=')){requestURL=word.slice(6);}
  else if(word==='-b'||word==='--cookie'){cookie=words[++i]||'';}
  else if(word.startsWith('--cookie=')){cookie=word.slice(9);}
  else if(word==='-H'||word==='--header'){
   const header=words[++i]||'';
   if(/^cookie\s*:/i.test(header))headerCookie=header.replace(/^cookie\s*:\s*/i,'');
  }else if(/^https?:\\?\/\//i.test(word)&&!requestURL)requestURL=word;
 }
 let url;
 try{url=new URL(requestURL.replace(/\\:/g,':'));}catch{throw new Error('The cURL command needs a valid TeraBox /api/list URL.');}
 if(url.protocol!=='https:'||!['www.terabox.com','dm.terabox.com','dm.terabox.app'].includes(url.hostname)||url.pathname!=='/api/list'){
  throw new Error('Use a cURL command for /api/list on www.terabox.com, dm.terabox.com or dm.terabox.app.');
 }
 const jsToken=url.searchParams.get('jsToken');
 if(!cookie&&!headerCookie)throw new Error('The cURL command has no Cookie value (-b or Cookie header).');
 if(!jsToken)throw new Error('The cURL URL has no jsToken value.');
 return {cookie:cookie||headerCookie,jsToken,host:url.hostname,bdstoken:url.searchParams.get('bdstoken')||''};
}
