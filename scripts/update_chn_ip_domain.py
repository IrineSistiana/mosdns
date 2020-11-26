import netaddr
import requests
import logging
import math

logger = logging.getLogger(__name__)


def update_ip_list():
    url = 'https://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest'
    timeout = 30
    save_to_file = './chn_ip.list'

    logger.info(f'fetching chn ip data from {url}')

    ipNetwork_list = []

    with requests.get(url, timeout=timeout) as res:
        if res.status_code != 200:
            raise Exception(f'status code :{res.status_code}')

        logger.info(f'parsing...')

        lines = res.text.splitlines()
        for line in lines:
            try:
                if line.find('|CN|ipv4|') != -1:
                    elems = line.split('|')
                    ip_start = elems[3]
                    count = int(elems[4])
                    cidr_prefix_length = int(32 - math.log(count, 2))
                    ipNetwork_list.append(netaddr.IPNetwork(f'{ip_start}/{cidr_prefix_length}\n'))

                if line.find('|CN|ipv6|') != -1:
                    elems = line.split('|')
                    ip_start = elems[3]
                    cidr_prefix_length = elems[4]
                    ipNetwork_list.append(netaddr.IPNetwork(f'{ip_start}/{cidr_prefix_length}\n'))
            except IndexError:
                logging.warning(f'unexpected format: {line}')

    logger.info('merging')
    ipNetwork_list = netaddr.cidr_merge(ipNetwork_list)
    logger.info('writing to file')

    with open(save_to_file, 'wt') as f:
        f.writelines([f'{x}\n' for x in ipNetwork_list])

    logger.info('all done')


def update_chn_domain_list():
    def get_domains_from(url: str, timeout=30):
        logger.info(f'fetching {url}')

        domains = []
        with requests.get(url, timeout=timeout) as res:
            if res.status_code != 200:
                res.close()
                raise Exception(f'status code :{res.status_code}')

            lines = res.text.splitlines()
            for line in lines:
                try:
                    if line.find('server=/') != -1:
                        elems = line.split('/')
                        domain = elems[1]
                        domains.append(domain)
                except IndexError:
                    logger.warning(f'unexpected format: {line}')

        return domains

    urls = ['https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf',
            'https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/google.china.conf',
            'https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/apple.china.conf']

    save_to = './chn_domain.list'
    domains = []

    for url in urls:
        domains = domains + get_domains_from(url)

    with open(save_to, 'wt') as f:
        f.writelines([f'{x}\n' for x in domains])

    logger.info('all done')


def download_chn_blocked_domain_list():
    url = 'https://github.com/Loyalsoldier/cn-blocked-domain/raw/release/domains.txt'
    timeout = 30
    save_to_file = './non_chn_domain.list'

    logger.info(f'fetching chn blocked domain from {url}')

    with requests.get(url, timeout=timeout) as res:
        if res.status_code != 200:
            res.close()
            raise Exception(f'status code :{res.status_code}')

        with open(save_to_file, 'wt') as f:
            f.write(res.text)


def update_all():
    update_chn_domain_list()
    download_chn_blocked_domain_list()
    update_ip_list()


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO)
    update_all()
