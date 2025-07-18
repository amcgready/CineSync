/**
 * Fanart.tv Banner Fetcher - Optimized for 1000x185 Banner Images
 * 
 * This utility fetches proper banner images (1000x185) from Fanart.tv
 * specifically designed for topbar/header use in CineSync.
 * 
 * Image Type Priority (PROPER BANNERS):
 * 1. moviebanner - Movie banners (1000x185) - designed for header use
 * 2. tvbanner - TV show banners (1000x185) - designed for header use
 * 
 * These banner types are specifically created for header/topbar use
 * with the perfect 1000x185 aspect ratio (5.4:1) - logos included are fine.
 * 
 * NO TMDB FALLBACK - maintains banner-specific image standards
 */

interface FanartBannerResult {
  url: string;
  type: string;
  title: string;
  source: 'fanart';
}

/**
 * Simple banner fetcher focused on textless images
 */
export const fetchFanartBanner = async (fanartApiKey: string) => {
  console.log('🎯 Starting textless Fanart.tv banner fetch...');
  
  // Test with known good movies that have textless backgrounds
  const testMovies = [
    { id: 550, name: 'Fight Club' },
    { id: 155, name: 'The Dark Knight' }, 
    { id: 680, name: 'Pulp Fiction' },
    { id: 424, name: "Schindler's List" },
    { id: 13, name: 'Forrest Gump' },
    { id: 278, name: 'The Shawshank Redemption' }
  ];
  
  for (const movie of testMovies) {
    try {
      console.log(`🎬 Testing ${movie.name} (ID: ${movie.id}) for textless banners`);
      
      const response = await fetch(`https://webservice.fanart.tv/v3/movies/${movie.id}?api_key=${fanartApiKey}`);
      
      if (response.ok) {
        const data = await response.json();
        console.log(`✅ Data received for ${data.name}`);
        
        // Prioritize ONLY true 1000x185 banner images
        const bannerTypes = ['moviebanner']; // ONLY 1000x185 movie banners
        
        for (const bannerType of bannerTypes) {
          if (data[bannerType] && data[bannerType].length > 0) {
            const banner = data[bannerType][0];
            console.log(`🎨 Found textless ${bannerType} banner:`, banner.url);
            return {
              url: banner.url,
              source: 'fanart',
              movie: data.name,
              bannerType,
              isTextless: ['moviebackground', 'moviethumb'].includes(bannerType)
            };
          }
        }
        
        console.log(`⚠️ No textless banners found for ${movie.name}`);
        console.log('Available types:', Object.keys(data).filter(key => 
          Array.isArray(data[key]) && data[key].length > 0
        ));
      } else {
        console.log(`❌ Request failed for ${movie.name}: ${response.status}`);
      }
    } catch (error) {
      console.log(`💥 Error for ${movie.name}:`, error);
    }
  }
  
  console.log('🔄 No textless Fanart.tv banners found - will use clean fallback background');
  return null;
};

/**
 * Fetches a textless banner for a specific movie or TV show from Fanart.tv ONLY
 * NO TMDB fallback to maintain strict textless standards
 */
export async function fetchTextlessBanner(
  tmdbId: number, 
  mediaType: 'movie' | 'tv',
  fanartApiKey: string
): Promise<FanartBannerResult | null> {
  
  // Only try Fanart.tv for guaranteed textless backgrounds
  if (fanartApiKey && fanartApiKey !== 'your_fanart_api_key_here') {
    try {
      const fanartUrl = mediaType === 'movie' 
        ? `https://webservice.fanart.tv/v3/movies/${tmdbId}?api_key=${fanartApiKey}`
        : `https://webservice.fanart.tv/v3/tv/${tmdbId}?api_key=${fanartApiKey}`;
      
      const response = await fetch(fanartUrl);
      
      if (response.ok) {
        const data = await response.json();
        
        // Prioritize ONLY true 1000x185 banner images
        const bannerTypes = mediaType === 'movie' 
          ? ['moviebanner']     // ONLY 1000x185 movie banners
          : ['tvbanner'];       // ONLY 1000x185 TV banners
        
        for (const imageType of bannerTypes) {
          if (data[imageType] && data[imageType].length > 0) {
            const selectedImage = data[imageType][Math.floor(Math.random() * data[imageType].length)];
            
            return {
              url: selectedImage.url,
              type: imageType,
              title: data.name || `${mediaType} ${tmdbId}`,
              source: 'fanart'
            };
          }
        }
        
        console.log(`No textless banners found for ${data.name || tmdbId} - available types:`, 
          Object.keys(data).filter(key => Array.isArray(data[key]) && data[key].length > 0)
        );
      }
    } catch (error) {
      console.debug('Fanart.tv request failed:', error);
    }
  }
  
  // No TMDB fallback - return null for clean gradient fallback
  return null;
}
