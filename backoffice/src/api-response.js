export async function readAPIResponse(response) {
 try {
  const result=await response.json();
  if(result&&typeof result==='object'&&!Array.isArray(result))return result;
 } catch {
  // Gateways can return an HTML error page instead of the backend's JSON.
 }
 if(response.ok)throw new Error('The server returned an invalid response. Refresh and try again.');
 if(response.status===502||response.status===504){
  return {error:`Gateway request failed (HTTP ${response.status}). The backend may be unavailable or the scan may have timed out. Check backend and proxy logs before retrying.`};
 }
 return {error:`Request failed (HTTP ${response.status}). Please try again.`};
}
