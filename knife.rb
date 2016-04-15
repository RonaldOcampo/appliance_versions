current_dir = File.dirname __FILE__

#log_level                :info
log_location             STDOUT
node_name                'paas-ci'

client_key               "#{current_dir}/.chef/pem/paas-ci.pem"
validation_client_name   'chef-validator'
validation_key           "#{current_dir}/.chef/pem/validation.pem"
chef_server_url          'https://paas-chef.mia.ucloud.int/'
syntax_check_cache_path  "#{current_dir}/.chef/syntax_check_cache"
ssl_verify_mode          :verify_none
cookbook_path            ["#{current_dir}/cookbooks"]

cookbook_copyright       'Ultimate Software'
cookbook_email           'UCloud_Paas@ultimatesoftware.com'
data_bag_encrypt_version 2 

