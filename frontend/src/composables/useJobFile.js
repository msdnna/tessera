import { jobs as jobsApi } from '@/api'
import { handOffBlob } from '@/utils/blobHandoff'

// downloadJobFile pulls a finished export's file down and hands it to the user
// (#3068). Through the api client, not a link: the route is protected and the
// access token lives in memory only. Throws on a failed request.
export async function downloadJobFile(job) {
  const res = await jobsApi.download(job.run_id)
  const name = job.meta?.file_name || `export.${job.meta?.format || 'csv'}`
  return handOffBlob(res.data, name)
}
